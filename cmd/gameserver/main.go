package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hs-server/internal/config"
	"hs-server/internal/database"
	"hs-server/internal/game"
	"hs-server/internal/game/dbstore"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
	"hs-server/internal/session"
)

const requestTimeout = 5 * time.Second
const connectionIdleTimeout = 120 * time.Second
const connectionWriteTimeout = 15 * time.Second

func main() {
	cfg := config.Load()
	ln, err := net.Listen("tcp", cfg.GameBind)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	var svc *game.Service
	var rsaKey *rsa.PrivateKey
	{
		activityPath := cfg.ActivityScheduleFile
		if activityPath == "" {
			activityPath = filepath.Join(cfg.DataDir, "activity-schedules.json")
		}
		activityRaw, err := os.ReadFile(activityPath)
		if err != nil {
			log.Fatalf("运营活动日程读取失败：%v", err)
		}
		var schedules []game.ActivitySchedule
		decoder := json.NewDecoder(bytes.NewReader(activityRaw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&schedules); err != nil {
			log.Fatalf("运营活动日程解析失败：%v", err)
		}
		var trailing any
		if err = decoder.Decode(&trailing); err != io.EOF {
			log.Fatal("运营活动日程尾部包含多余内容")
		}
		if err = game.SetActivitySchedules(schedules); err != nil {
			log.Fatal(err)
		}
		log.Printf("运营活动日程已加载：%d项", len(schedules))
		var catalog hotfix.Catalog
		if err := catalog.Load(filepath.Join(cfg.DataDir, "hotfix.json")); err != nil {
			log.Fatal(err)
		}
		var accounts game.Accounts
		if cfg.DatabaseURL != "" {
			// 真实数据库优先（HS_DATABASE_URL），建表失败即退出避免假在线。
			dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			pool, err := database.OpenPostgres(dbCtx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
			cancel()
			if err != nil {
				log.Fatal(err)
			}
			defer pool.Close()
			store := dbstore.New(pool.Pool())
			schemaCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err = store.EnsureSchema(schemaCtx)
			cancel()
			if err != nil {
				log.Fatal(err)
			}
			accounts = store
			log.Printf("账号存储：PostgreSQL")
		} else {
			fixtures, err := game.LoadFixtures(filepath.Join(cfg.DataDir, "accounts.dev.json"))
			if err != nil {
				log.Fatal(err)
			}
			accounts = fixtures
			log.Printf("账号存储：开发夹具（未配置 HS_DATABASE_URL）")
		}
		svc = game.New(accounts, &catalog)
		if rsaKey, err = loadPrivateKey(cfg.RSAKeyPath); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.DebugBind != "" {
		host, _, err := net.SplitHostPort(cfg.DebugBind)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			log.Fatal("HS_GAME_DEBUG_BIND 必须绑定本机回环 IP")
		}
		debug := &http.Server{Addr: cfg.DebugBind, Handler: game.DebugHandler(svc, cfg.MaxSessions), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
		go func() { log.Fatal(debug.ListenAndServe()) }()
		log.Printf("本机业务演练接口监听 %s", cfg.DebugBind)
	}
	if cfg.AdminBind != "" {
		host, _, err := net.SplitHostPort(cfg.AdminBind)
		if err != nil || !net.ParseIP(host).IsLoopback() || len(cfg.AdminToken) < 32 {
			log.Fatal("管理接口必须绑定回环 IP，且 HS_GAME_ADMIN_TOKEN 至少32字节")
		}
		admin := &http.Server{Addr: cfg.AdminBind, Handler: game.AdminHandler(svc, cfg.AdminToken), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
		go func() { log.Fatal(admin.ListenAndServe()) }()
		log.Printf("本机管理接口监听 %s", cfg.AdminBind)
	}
	if rsaKey == nil {
		log.Printf("HS_GAME_RSA_KEY 未配置：session_key 将按明文 SessionKey 解析（仅限联调）")
	}
	mgr := session.NewManager(cfg.MaxSessions)
	log.Printf("游戏服务监听 %s，会话上限 %d，加密=%v", cfg.GameBind, cfg.MaxSessions, rsaKey != nil)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go serve(conn, mgr, svc, rsaKey, cfg.MaxFramePayload)
	}
}

// serve 承载单条连接：MobileRPC 帧 ↔ RC4 门 ↔ 传输适配 ↔ 业务层。
func serve(conn net.Conn, mgr *session.Manager, svc *game.Service, key *rsa.PrivateKey, maxPayload int) {
	serveWithIdleTimeout(conn, mgr, svc, key, maxPayload, connectionIdleTimeout)
}

// 空闲期限只约束两次完整帧之间的等待；有心跳的长连接可持续在线。
// 单独限制写入，避免客户端停止读取时占住会话；测试可缩短空闲期限。
func serveWithIdleTimeout(conn net.Conn, mgr *session.Manager, svc *game.Service, key *rsa.PrivateKey, maxPayload int, idle time.Duration) {
	defer conn.Close()
	s, err := mgr.New(conn.RemoteAddr().String())
	if err != nil {
		return
	}
	defer mgr.Remove(s.ID)
	_ = conn.SetReadDeadline(time.Now().Add(idle))
	g := newGate(maxPayload, func(format string, v ...any) { log.Printf("连接 %s "+format, append([]any{s.ID}, v...)...) })
	biz := game.NewConnection()
	defer svc.Detach(biz)
	var mu sync.Mutex
	buf := make([]byte, 0, 64*1024)
	read := make([]byte, 32*1024)
	writeFrames := func(frames []mobileproto.Frame) bool {
		// 整批加密和写入必须同序，定时推送不能交换RC4流中的帧顺序。
		mu.Lock()
		defer mu.Unlock()
		for _, f := range frames {
			wire, encErr := mobileproto.Encode(f, maxPayload)
			if encErr != nil {
				log.Printf("连接 %s 编码失败: %v", s.ID, encErr)
				return false
			}
			g.mu.Lock()
			if g.write != nil {
				enc := make([]byte, len(wire))
				g.write.xor(enc, wire)
				wire = enc
			}
			g.mu.Unlock()
			_ = conn.SetWriteDeadline(time.Now().Add(connectionWriteTimeout))
			_, err := conn.Write(wire)
			if err != nil {
				return false
			}
		}
		return true
	}
	clockCtx, stopClock := context.WithCancel(context.Background())
	defer stopClock()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-clockCtx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(clockCtx, requestTimeout)
				pushes, err := svc.Tick(ctx, biz)
				cancel()
				if err != nil {
					log.Printf("连接 %s 战斗时钟失败: %v", s.ID, err)
					continue
				}
				if len(pushes) > 0 && !writeFrames(g.encodePushes(pushes)) {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	for {
		n, err := conn.Read(read)
		if n > 0 {
			g.mu.Lock()
			if g.read != nil {
				clear := make([]byte, n)
				g.read.xor(clear, read[:n])
				buf = append(buf, clear...)
			} else {
				buf = append(buf, read[:n]...)
			}
			g.mu.Unlock()
		}
		for len(buf) > 0 {
			f, used, complete, decErr := mobileproto.Decode(buf, maxPayload)
			if decErr != nil {
				return
			}
			if !complete {
				break
			}
			buf = buf[used:]
			// 完整帧到达才续期，半帧慢速传输不能无限延长连接。
			_ = conn.SetReadDeadline(time.Now().Add(idle))
			if !writeFrames(g.handleFrame(svc, biz, key, f)) {
				return
			}
		}
		if err != nil {
			log.Printf("连接 %s 读取结束: %v", s.ID, err)
			return
		}
	}
}
