package sdk

import (
	"crypto/md5"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// protocolDefaultHTML 是打包内 assets/unisdk_protocol_default_txt 的原样字节
// （NTUniSDK 内置的网易游戏用户协议全文，2026-10-06 从 hs-direct.apk 提取）。
// ProtocolLauncher 的 ProtocolProvider 通过 getRequestUrl() 下载协议 JSON
// （dex 内 https://service0000002.r18sex.net/html/latest_v89.json）后，
// 再按 FullTextBase64HttpsUrl 取 base64 全文并用 Hash=md5(文本) 校验。
//
//go:embed protocol_default.html
var protocolDefaultHTML []byte

var (
	protocolTextB64 = base64.StdEncoding.EncodeToString(protocolDefaultHTML)
	protocolTextMD5 = func() string {
		sum := md5.Sum([]byte(protocolTextB64))
		return hex.EncodeToString(sum[:])
	}()
)

// Telemetry 埋点接收:按天落盘 jsonl,行含 host/path/UA 与请求体。
type Telemetry struct {
	Dir string

	mu sync.Mutex
}

func (t *Telemetry) record(r *http.Request, body []byte) {
	entry := map[string]any{
		"time":     time.Now().UTC().Format(time.RFC3339Nano),
		"method":   r.Method,
		"host":     r.Host,
		"path":     r.URL.Path,
		"query":    r.URL.RawQuery,
		"ua":       r.UserAgent(),
		"body_len": len(body),
	}
	if len(body) > 0 {
		trimmed := body
		if len(trimmed) > 32<<10 {
			trimmed = trimmed[:32<<10]
		}
		entry["body"] = string(trimmed)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.MkdirAll(t.Dir, 0700); err != nil {
		return
	}
	name := filepath.Join(t.Dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("埋点落盘失败 %s: %v", name, err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// HTTPHandler 提供自建 SDK 端点与埋点接收。
// 客户端域名重写后:sdk_logic/sdk_mgr.py 指向 http://sdk.r18sex.net:8080,
// 埋点(applog/hubble/渠道 ad.*)同服;未知路径一律空 200 防 SDK 重试死循环
// (契约同 internal/hotfix 兜底,证据:hotfix_test.go 2026-10-05 实测注释)。
func HTTPHandler(telemetryDir string) http.Handler {
	tele := &Telemetry{Dir: telemetryDir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	empty := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "{}")
	}
	// mgbsdk/unisdk 初始化与版本检查端点
	for _, p := range []string{"/h62/sdk", "/h62/sdk/", "/unisdk/"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			log.Printf("SDK 请求 %s %s%s from %s", r.Method, r.Host, r.URL.Path, r.RemoteAddr)
			empty(w)
		})
	}
	// gmsdk web 容器页(entities/components/gmsdk_mgr.py 的 token 页)
	gmPage := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>gmsdk</title>")
	}
	mux.HandleFunc("/gm", gmPage)
	mux.HandleFunc("/gm/", gmPage)

	// NTUniSDK 协议配置（ProtocolLauncher 启动必经）：
	// getRequestUrl()=https://service0000002.r18sex.net/html/latest_v89.json；
	// parseLocalProtocol 要求根对象含 VersionId{Id,Version}，否则本地协议解析失败报
	// “加载用户协议失败”；全文经 FullTextBase64HttpsUrl 下载并以 Hash=md5(文本) 校验。
	protocolJSON := func() []byte {
		body, _ := json.Marshal(map[string]any{
			"Id":                           1,
			"VersionId":                    map[string]any{"Id": 1, "Version": 1},
			"IsMinorChange":                false,
			"PrevMajorChangeId":            0,
			"FullTextBase64HttpsUrl":       "https://service0000002.r18sex.net/html/agreement_b64.txt",
			"FullTextUpdateBase64HttpsUrl": "",
			"Hash":                         protocolTextMD5,
		})
		return body
	}()
	protocolHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(protocolJSON)
	}
	mux.HandleFunc("/html/latest_v89.json", protocolHandler)
	agreementHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, protocolTextB64)
	}
	mux.HandleFunc("/html/agreement_b64.txt", agreementHandler)
	// 埋点接收:applog(matrix)与其余上报路径
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		tele.record(r, body)
		if strings.HasPrefix(r.URL.Path, "/applog") || strings.Contains(r.URL.Path, "log") {
			log.Printf("埋点 %s %s%s (%dB)", r.Method, r.Host, r.URL.Path, len(body))
		} else {
			log.Printf("SDK 兜底 %s %s%s from %s", r.Method, r.Host, r.URL.Path, r.RemoteAddr)
		}
		empty(w)
	})
	return mux
}
