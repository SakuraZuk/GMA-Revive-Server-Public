package game

import (
	_ "embed"
	"net/http"
)

//go:embed admin_ui.html
var adminUI []byte

//go:embed admin_ui.js
var adminUIJS []byte

// 页面为无数据静态资源；所有管理数据和写操作仍要求Bearer。
func serveAdminUI(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" {
		return false
	}
	var body []byte
	switch r.URL.Path {
	case "/admin", "/admin/":
		body = adminUI
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case "/admin/ui.js":
		body = adminUIJS
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	default:
		return false
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
	return true
}
