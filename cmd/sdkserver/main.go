package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"hs-server/internal/config"
	"hs-server/internal/sdk"
)

func main() {
	cfg := config.Load()
	log.SetOutput(os.Stdout)
	h := http.NewServeMux()
	h.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h.HandleFunc("/v1/route", func(w http.ResponseWriter, r *http.Request) {
		// 本接口是项目管理路由，不是客户端原生 SDK 登录协议。
		version := r.URL.Query().Get("version")
		route := sdk.SelectRoute(version, cfg.HotfixVersions, cfg.GameAddress, cfg.HotfixAddress, os.Getenv("HS_HOTFIX_REQUIRED") == "1")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(route)
	})
	// 客户端原生 SDK 端点与埋点接收(域名重写后指向 hsqsl-sdk/hsqsl-log.r18sex.net:8080)
	h.Handle("/", sdk.HTTPHandler(filepath.Join(cfg.DataDir, "telemetry")))
	log.Printf("SDK 管理路由监听 %s(含埋点接收)", cfg.SDKBind)
	server := &http.Server{Addr: cfg.SDKBind, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	// dex 域名重写后 *.r18sex.net 默认 443:Mpay/SDK HTTPS 入口在本机
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		go func() {
			tlsSrv := &http.Server{Addr: ":443", Handler: h,
				ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
				WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
			log.Printf("SDK HTTPS(Mpay 等 *.r18sex.net)监听 :443")
			log.Fatal(tlsSrv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey))
		}()
	}
	log.Fatal(server.ListenAndServe())
}
