// hotfixserver 独立部署到热更服务器，承担 HTTPS、资源下载及项目假 DNS。
package main

import (
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"hs-server/internal/config"
	"hs-server/internal/hotfix"
)

func main() {
	cfg := config.Load()
	var catalog hotfix.Catalog
	if err := catalog.Load(filepath.Join(cfg.DataDir, "hotfix.json")); err != nil {
		log.Fatal(err)
	}
	handler, err := hotfix.Handler(&catalog, cfg.DataDir, cfg.GameAddress, cfg.DNSAnswer, cfg.GameDomain, cfg.GameDNSAnswer)
	if err != nil {
		log.Fatal(err)
	}
	// 先绑定所有配置端口；任何冲突都退出，不影响既有服务。
	httpListener, err := net.Listen("tcp", cfg.HotfixBind)
	if err != nil {
		log.Fatal(err)
	}
	defer httpListener.Close()
	var tlsListener net.Listener
	if cfg.HotfixTLSBind != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			log.Fatal("HTTPS 必须设置 HS_TLS_CERT 和 HS_TLS_KEY")
		}
		tlsListener, err = net.Listen("tcp", cfg.HotfixTLSBind)
		if err != nil {
			log.Fatal(err)
		}
		defer tlsListener.Close()
	}
	if cfg.DNSBind != "" {
		answer := net.ParseIP(cfg.DNSAnswer)
		gameAnswer := net.ParseIP(cfg.GameDNSAnswer)
		if answer.To4() == nil || gameAnswer.To4() == nil {
			log.Fatal("HS_DNS_ANSWER 必须为 IPv4")
		}
		addr, err := net.ResolveUDPAddr("udp", cfg.DNSBind)
		if err != nil {
			log.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()
		answers := map[string]net.IP{
			"netease.com":  answer,
			"easebar.com":  answer,
			cfg.GameDomain: gameAnswer,
		}
		go func() { log.Fatal(hotfix.ServeDNSWithAnswers(conn, answers)) }()
		tcpLn, tcpErr := net.Listen("tcp", cfg.DNSBind)
		if tcpErr == nil {
			defer tcpLn.Close()
			go func() { log.Fatal(hotfix.ServeTCPDNS(tcpLn, answers)) }()
		} else {
			log.Printf("热更 TCP DNS 未监听：%v", tcpErr)
		}
		log.Printf("热更 DNS 监听 %s（UDP%s），热更别名返回 %s，游戏域名 *.%s 返回 %s", cfg.DNSBind, map[bool]string{true: "+TCP"}[tcpErr == nil], cfg.DNSAnswer, cfg.GameDomain, cfg.GameDNSAnswer)
	}
	server := func() *http.Server {
		return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second}
	}
	if tlsListener != nil {
		go func() { log.Fatal(server().ServeTLS(tlsListener, cfg.TLSCert, cfg.TLSKey)) }()
		log.Printf("热更 HTTPS 监听 %s", cfg.HotfixTLSBind)
	}
	log.Printf("热更 HTTP 监听 %s，游戏地址 %s", cfg.HotfixBind, cfg.GameAddress)
	log.Fatal(server().Serve(httpListener))
}
