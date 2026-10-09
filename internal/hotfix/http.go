package hotfix

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler 只注册已观察到的热更、网关列表、公告及日志路由。
func Handler(catalog *Catalog, dataDir, gameAddress, dnsIP, gameDomain, gameIP string) (http.Handler, error) {
	netip, cfgGameDomain, cfgGameIP := dnsIP, gameDomain, gameIP
	if _, _, err := net.SplitHostPort(gameAddress); err != nil {
		return nil, fmt.Errorf("游戏地址需要包含端口：%w", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dataDir, "patch_list_pub_android.txt"))
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Android struct {
			Version   string            `json:"version"`
			Base      string            `json:"base_version"`
			NPK       map[string]string `json:"npk"`
			PatchPath string            `json:"0patchpath"`
		} `json:"android"`
	}
	if err = json.Unmarshal(manifest, &parsed); err != nil {
		return nil, err
	}
	if parsed.Android.Version == "" || parsed.Android.Base == "" || parsed.Android.NPK == nil || parsed.Android.PatchPath == "" {
		return nil, fmt.Errorf("Android 补丁清单缺少版本、基础版本、资源表或下载路径")
	}
	notice, err := os.ReadFile(filepath.Join(dataDir, "notice.xml"))
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(strings.NewReader(string(notice)))
	for {
		_, err = decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("公告 XML 不合法：%w", err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "正常\n") })
	mux.HandleFunc("GET /pl/patch_hotfix_data_pub", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, catalog.Startup())
	})
	mux.HandleFunc("GET /pl/patch_list_pub_android.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(manifest)
	})
	// listsvr 替换后的实际 URL(patch_utils: /patch_list/<file>)
	mux.HandleFunc("GET /patch_list/patch_list_pub_android.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(manifest)
	})
	mux.HandleFunc("GET /server_list_public.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// 客户端 server_list_mgr._purge_server_info 指令级还原：空格分列、
		// gate 列 = cols[nettype+8]（两段 ip:port 即可）、末行必须 network=xx。
		endpoint := gameAddress
		_, _ = fmt.Fprintf(w, "ANDIOSNETEASE 10001 1 5 1 3 明日生机 明日生机 %s %s %s %s NETEASE 2020.12.17 10:00\nnetwork=bgp\n", endpoint, endpoint, endpoint, endpoint)
	})
	httpdns := func(w http.ResponseWriter, r *http.Request) {
		// 网易 HttpDNS v2：patch_logic/httpdns_resolver.py 指令级还原的响应契约。
		domain := r.URL.Query().Get("domain")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		answer := net.ParseIP(netip).To4()
		if strings.HasSuffix(domain, "."+cfgGameDomain) || domain == cfgGameDomain {
			answer = net.ParseIP(cfgGameIP).To4()
		}
		if answer == nil || domain == "" {
			_, _ = io.WriteString(w, "{\"status\":1}")
			return
		}
		_, _ = fmt.Fprintf(w, "{\"status\":0,\"domain\":%q,\"addrs\":[\"%s\"],\"ttl\":300}", domain, answer)
	}
	mux.HandleFunc("/v2/", httpdns)
	// hdserver 入口：客户端 httpdns_resolver 的 dns.update.* 域名路径,契约同 v2。
	mux.HandleFunc("/hdserver", httpdns)
	mux.HandleFunc("GET /game_notice/notice_formal", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write(notice)
	})
	// FileServer 提供标准 Range/HEAD；文件目录仅用于公开热更资源。
	mux.Handle("/resources/", http.StripPrefix("/resources/", http.FileServer(http.Dir(filepath.Join(dataDir, "resources")))))
	for _, path := range []string{"/applog", "/appdump", "/upload_patch_log"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" && r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				http.Error(w, "日志请求过大或读取失败", http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}
	// 通配兜底：SDK/分析类未知路径返回空 200，避免客户端重试弹窗。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := make([]byte, 16<<10)
		n, _ := io.ReadFull(r.Body, body)
		if r.Method == "POST" && n > 0 {
			log.Printf("兜底请求体 %s%s (%dB): %s", r.Host, r.URL.Path, n, string(body[:n]))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "{}")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 客户端域名重写后 URL 带项目前缀(/h62/、/h62na/、/h62jp/ 等),
		// 剥离后再路由,保持 /server_list_public.txt 等契约不变。
		for _, p := range []string{"/h62", "/h62na", "/h62jp"} {
			if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, p)
				if r.URL.Path == "" {
					r.URL.Path = "/"
				}
				break
			}
		}
		log.Printf("热更请求 %s %s%s%s from %s", r.Method, r.Host, r.URL.Path, r.URL.RawQuery, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	}), nil
}
