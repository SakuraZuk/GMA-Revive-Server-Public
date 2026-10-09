package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"hs-server/internal/config"
	"hs-server/internal/session"
)

type loginResponse struct {
	SessionID string `json:"session_id"`
	Game      string `json:"game"`
}

func main() {
	cfg := config.Load()
	mgr := session.NewManager(cfg.MaxSessions)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	// 项目管理接口，仅创建临时会话，不是网易账号鉴权或游戏协议登录。
	mux.HandleFunc("POST /v1/login", func(w http.ResponseWriter, r *http.Request) {
		mgr.ExpireIdle(time.Now().Add(-5 * time.Minute))
		s, err := mgr.New(r.RemoteAddr)
		if err != nil {
			http.Error(w, "临时会话容量已满", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{SessionID: s.ID, Game: cfg.GameAddress})
	})
	log.SetOutput(os.Stdout)
	log.Printf("登录管理接口监听 %s，会话上限 %d", cfg.LoginBind, cfg.MaxSessions)
	server := &http.Server{Addr: cfg.LoginBind, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	log.Fatal(server.ListenAndServe())
}
