package sdk

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPHandlerSDKAndTelemetry(t *testing.T) {
	dir := t.TempDir()
	h := HTTPHandler(filepath.Join(dir, "telemetry"))
	// SDK 端点空 200
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/h62/sdk", nil))
	if w.Code != 200 || w.Body.String() != "{}" {
		t.Fatalf("SDK 端点应空 200: %d %q", w.Code, w.Body.String())
	}
	// 埋点 POST 落盘
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/applog/open_log?channel=netease", bytes.NewReader([]byte(`{"e":"test"}`)))
	h.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "{}" {
		t.Fatalf("埋点应 200: %d %q", w.Code, w.Body.String())
	}
	files, err := os.ReadDir(filepath.Join(dir, "telemetry"))
	if err != nil || len(files) != 1 {
		t.Fatalf("埋点未落盘: %v %d", err, len(files))
	}
	body, _ := os.ReadFile(filepath.Join(dir, "telemetry", files[0].Name()))
	if !strings.Contains(string(body), `"path":"/applog/open_log"`) || !strings.Contains(string(body), `\"e\":\"test\"`) {
		t.Fatalf("埋点内容缺失: %s", body)
	}
	// gmsdk 容器页
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/gm/menu?token=x", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "gmsdk") {
		t.Fatalf("gmsdk 页错误: %d %q", w.Code, w.Body.String())
	}
	// 根路径 404,未知路径空 200
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 404 {
		t.Fatalf("根路径应 404: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/unisdk/whatever", bytes.NewReader([]byte(`{}`))))
	if w.Code != 200 || w.Body.String() != "{}" {
		t.Fatalf("未知 SDK 路径应空 200: %d %q", w.Code, w.Body.String())
	}
}
