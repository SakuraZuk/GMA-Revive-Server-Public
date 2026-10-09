package game

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// RuneAdminAudit 与发放结果同事务保存；操作员为共用令牌下填写的审计标签。
type RuneAdminAudit struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
	Time     int64  `json:"time"`
	RuneUUID string `json:"rune_uuid"`
}

// AdminHandler 为独立 HTTP 管理接口，不调用客户端 RPC、不创建登录会话。
func AdminHandler(service *Service, token string) http.Handler {
	mux := http.NewServeMux()
	registerMailAdmin(mux, service)
	registerPlayerAdmin(mux, service)
	mux.HandleFunc("POST /admin/runes/grant", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			AvatarOID string   `json:"avatar_oid"`
			Receipt   string   `json:"receipt"`
			Operator  string   `json:"operator"`
			Reason    string   `json:"reason"`
			Spec      RuneSpec `json:"spec"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "请求 JSON 无效或包含多余字段", http.StatusBadRequest)
			return
		}
		if !validObjectID(request.AvatarOID) || request.Receipt == "" || len(request.Receipt) > 100 || !adminAuditText(request.Operator, 64) || !adminAuditText(request.Reason, 512) || !adminAuditText(request.Receipt, 100) {
			http.Error(w, "角色、凭据或审计字段无效", http.StatusBadRequest)
			return
		}
		oid, _ := hex.DecodeString(request.AvatarOID)
		store, ok := service.Accounts.(ProgressAccounts)
		if !ok {
			http.Error(w, "账号存储不支持管理事务", http.StatusServiceUnavailable)
			return
		}
		key := "admin:" + request.Receipt
		var granted Rune
		var grantErr error
		replayed := false
		now := service.Now()
		_, err := store.UpdateProgress(r.Context(), oid, func(p *Progress) error {
			if err := MigrateRunes(p); err != nil {
				return err
			}
			if audit, exists := p.RuneAdminAudit[key]; exists {
				if audit.Operator != request.Operator || audit.Reason != request.Reason {
					grantErr = errors.New("同一凭据的审计内容冲突")
					return grantErr
				}
			}
			_, replayed = p.RuneGrantReceipts[key]
			granted, grantErr = GrantRuneOnce(p, key, request.Spec, now)
			if grantErr != nil {
				return grantErr
			}
			if p.RuneAdminAudit == nil {
				p.RuneAdminAudit = map[string]RuneAdminAudit{}
			}
			if _, exists := p.RuneAdminAudit[key]; !exists {
				p.RuneAdminAudit[key] = RuneAdminAudit{Operator: request.Operator, Reason: request.Reason, Time: now.Unix(), RuneUUID: granted.UUID}
			}
			return nil
		})
		if err != nil {
			if grantErr != nil {
				http.Error(w, grantErr.Error(), http.StatusUnprocessableEntity)
			} else {
				log.Printf("管理发契印事务失败：%v", err)
				http.Error(w, "管理事务失败，未发放", http.StatusInternalServerError)
			}
			return
		}
		if !replayed {
			service.publishPlayerRefresh(oid)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !replayed {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"状态": "成功", "重复请求": replayed, "rune": runeProperties(granted), "同步方式": "在线连接下一次Tick读取最新存档；离线玩家重登录"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			http.Error(w, "管理接口仅允许本机回环访问", http.StatusForbidden)
			return
		}
		if serveAdminUI(w, r) {
			return
		}
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "管理令牌无效", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func adminAuditText(text string, max int) bool {
	return len(text) <= max && strings.TrimSpace(text) != "" && !strings.ContainsAny(text, "\r\n\x00")
}
