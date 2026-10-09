package game

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"hs-server/internal/session"
)

// DebugHandler 是本机 JSON 业务演练接口，不是 Android 客户端协议。
func DebugHandler(service *Service, maxSessions int) http.Handler {
	manager := session.NewManager(maxSessions)
	var mu sync.Mutex
	type entry struct {
		connection *Connection
		touched    time.Time
	}
	connections := map[string]entry{}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]string{"状态": "正常", "模式": "本机业务演练"})
	})
	mux.HandleFunc("POST /debug/session", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		for id, item := range connections {
			if time.Since(item.touched) > 5*time.Minute {
				delete(connections, id)
				manager.Remove(id)
			}
		}
		s, err := manager.New(r.RemoteAddr)
		if err != nil {
			http.Error(w, "调试会话已满", http.StatusServiceUnavailable)
			return
		}
		connections[s.ID] = entry{connection: NewConnection(), touched: time.Now()}
		write(w, map[string]string{"session_id": s.ID})
	})
	for _, endpoint := range []string{"POST /debug/rpc", "POST /debug/become-player", "DELETE /debug/session"} {
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				SessionID string            `json:"session_id"`
				Method    string            `json:"method"`
				Args      []json.RawMessage `json:"args"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				http.Error(w, "请求 JSON 无效", http.StatusBadRequest)
				return
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				http.Error(w, "请求包含多余内容", http.StatusBadRequest)
				return
			}
			mu.Lock()
			item, ok := connections[request.SessionID]
			if ok && time.Since(item.touched) > 5*time.Minute {
				delete(connections, request.SessionID)
				manager.Remove(request.SessionID)
				ok = false
			}
			if ok {
				item.touched = time.Now()
				connections[request.SessionID] = item
			}
			if ok && r.Method == "DELETE" {
				delete(connections, request.SessionID)
				manager.Remove(request.SessionID)
			}
			mu.Unlock()
			if !ok {
				http.Error(w, "调试会话不存在或已过期", http.StatusNotFound)
				return
			}
			if r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			var pushes []Push
			var err error
			if r.URL.Path == "/debug/become-player" {
				pushes, err = service.BecomePlayer(item.connection)
			} else {
				pushes, err = service.Handle(r.Context(), item.connection, request.Method, request.Args)
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			write(w, map[string]any{"phase": item.connection.Phase(), "pushes": pushes})
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			http.Error(w, "调试接口仅允许本机访问", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
