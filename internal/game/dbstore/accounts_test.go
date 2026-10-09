package dbstore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"hs-server/internal/game"
)

// 每次测试独立 schema，只清理由测试自己创建的命名空间。
func testStore(t *testing.T) *Store {
	t.Helper()
	// 常规业务保持原初始资产；全英雄开关由专项用例分别验证开启和关闭。
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	url := os.Getenv("HS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未配置 HS_TEST_DATABASE_URL，跳过真实数据库验收")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	adminConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	// 在线玩家与实库核验共享48槽数据库；管理连接只用于建/删隔离schema。
	adminConfig.MaxConns = 1
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("hs_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("清理测试表失败：%v", err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	// 保留原并发操作数，以连接池排队控制测试竞争，避免挤占生产连接槽。
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := New(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("重复应用定义失败：%v", err)
	}
	return store
}

func TestPostgresAuthenticationAndPersistentAvatar(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "甲", Password: "测试密码", Hostnum: 10001}
	// 登录与注册分离：未知账号登录必须拒绝，显式注册是唯一建号入口。
	if _, err := s.QuickLogin(ctx, info); err == nil {
		t.Fatal("未知账号获准登录")
	}
	first, err := s.Register(ctx, info)
	if err != nil || len(first.Avatars) != 1 || len(first.Avatars[0].OID) != 12 || first.Avatars[0].Info.Nickname != "书灵甲" {
		t.Fatalf("显式注册失败：%+v %v", first, err)
	}
	if _, err = s.Register(ctx, info); err == nil {
		t.Fatal("重复注册未被拒绝")
	}
	var hash string
	var loginTime time.Time
	if err = s.pool.QueryRow(ctx, `SELECT password_hash,last_login_at FROM accounts WHERE account=$1`, info.Account).Scan(&hash, &loginTime); err != nil {
		t.Fatal(err)
	}
	if hash == info.Password || bcrypt.CompareHashAndPassword([]byte(hash), []byte(info.Password)) != nil {
		t.Fatal("密码未正确哈希")
	}
	wrong := info
	wrong.Password = "错误密码"
	if _, err = s.QuickLogin(ctx, wrong); err == nil {
		t.Fatal("错误密码获准登录")
	}
	var after time.Time
	_ = s.pool.QueryRow(ctx, `SELECT last_login_at FROM accounts WHERE account=$1`, info.Account).Scan(&after)
	if !loginTime.Equal(after) {
		t.Fatal("失败鉴权修改了登录时间")
	}
	if _, err = s.pool.Exec(ctx, `UPDATE avatars SET nickname='持久角色',level=8 WHERE account=$1`, info.Account); err != nil {
		t.Fatal(err)
	}
	// 模拟重建存储实例，角色编号和属性必须继续来自数据库。
	second, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || !bytes.Equal(first.Avatars[0].OID, second.Avatars[0].OID) || second.Avatars[0].Info.Nickname != "持久角色" || second.Avatars[0].Info.Level != 8 {
		t.Fatalf("持久化读取失败：%+v %v", second, err)
	}
	info.Hostnum = 10002
	other, err := s.QuickLogin(ctx, info)
	if err != nil || len(other.Avatars) != 2 || bytes.Equal(other.Avatars[0].OID, other.Avatars[1].OID) {
		t.Fatalf("跨服建角失败：%+v %v", other, err)
	}
}

func TestPostgresConcurrentRegistrationAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "并发测试", Password: "pw", Hostnum: 1}
	// 并发注册同一账号：恰好一个成功，其余必须拒绝且不建多余角色。
	var wg sync.WaitGroup
	results := make(chan game.Identity, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity, err := s.Register(ctx, info)
			if err != nil {
				failures <- err
			} else {
				results <- identity
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 1 {
		t.Fatalf("并发注册成功数=%d 应为 1", len(results))
	}
	for err := range failures {
		if err != game.ErrAccountExists {
			t.Errorf("并发注册冲突错误=%v 应为账号已存在", err)
		}
	}
	// 并发登录既有账号：全部成功且角色一致。
	loginResults := make(chan game.Identity, 8)
	loginFailures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity, err := s.QuickLogin(ctx, info)
			if err != nil {
				loginFailures <- err
			} else {
				loginResults <- identity
			}
		}()
	}
	wg.Wait()
	close(loginResults)
	close(loginFailures)
	for err := range loginFailures {
		t.Errorf("并发登录失败：%v", err)
	}
	var oid []byte
	for result := range loginResults {
		if len(result.Avatars) != 1 {
			t.Fatal("重复建角")
		}
		if oid == nil {
			oid = result.Avatars[0].OID
		}
		if !bytes.Equal(oid, result.Avatars[0].OID) {
			t.Fatal("同账号返回不同角色")
		}
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM avatars WHERE account=$1`, info.Account).Scan(&count); err != nil || count != 1 {
		t.Fatalf("角色数量=%d 错误=%v", count, err)
	}
	// 主动令建角失败，验证注册插入与建角一并回滚。
	if _, err := s.pool.Exec(ctx, `ALTER TABLE avatars ADD CONSTRAINT test_reject_host CHECK(hostnum<>999)`); err != nil {
		t.Fatal(err)
	}
	bad := game.ClientInfo{Account: "回滚测试", Password: "pw", Hostnum: 999}
	if _, err := s.Register(ctx, bad); err == nil {
		t.Fatal("故障注入未生效")
	}
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE account=$1`, bad.Account).Scan(&count)
	if count != 0 {
		t.Fatal("事务失败留下了半注册账号")
	}
	deadline, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.QuickLogin(deadline, info); err == nil {
		t.Fatal("已取消请求仍获准登录")
	}
}
