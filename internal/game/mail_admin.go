package game

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// 收据、审计与邮件同事务；邮件删除后仍保留收据，不重新发放。
type MailAdminReceipt struct {
	Fingerprint string `json:"fingerprint"`
	Operator    string `json:"operator"`
	Reason      string `json:"reason"`
	Time        int64  `json:"time"`
	MID         int    `json:"mid"`
	UUID        string `json:"uuid"`
}

func registerMailAdmin(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /admin/mail/issue", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AvatarOID string `json:"avatar_oid"`
			Receipt   string `json:"receipt"`
			Operator  string `json:"operator"`
			Reason    string `json:"reason"`
			Mail      Mail   `json:"mail"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF || !validObjectID(req.AvatarOID) || !adminAuditText(req.Receipt, 100) || !adminAuditText(req.Operator, 64) || !adminAuditText(req.Reason, 512) {
			http.Error(w, "发件 JSON、角色或审计字段无效", http.StatusBadRequest)
			return
		}
		if req.Mail.State != 0 || req.Mail.ModifiedAt != 0 {
			http.Error(w, "邮件状态和修改时间由服务端生成", http.StatusBadRequest)
			return
		}
		store, ok := s.Accounts.(ProgressAccounts)
		if !ok {
			http.Error(w, "存储不支持管理事务", http.StatusServiceUnavailable)
			return
		}
		payload, _ := json.Marshal(req)
		sum := sha256.Sum256(payload)
		fingerprint := hex.EncodeToString(sum[:])
		oid, _ := hex.DecodeString(req.AvatarOID)
		replayed := false
		var receipt MailAdminReceipt
		businessError := false
		p, err := store.UpdateProgress(r.Context(), oid, func(p *Progress) error {
			if old, ok := p.MailAdminReceipts[req.Receipt]; ok {
				if old.Fingerprint != fingerprint {
					businessError = true
					return errMailReceiptConflict
				}
				receipt = old
				replayed = true
				return nil
			}
			if len(p.MailAdminReceipts) >= 10000 {
				businessError = true
				return errMailReceiptFull
			}
			if err := s.prepareAndInsertMail(p, req.Mail); err != nil {
				businessError = true
				return err
			}
			m := p.ShortMailInfo[req.Mail.MID]
			receipt = MailAdminReceipt{Fingerprint: fingerprint, Operator: req.Operator, Reason: req.Reason, Time: s.Now().Unix(), MID: m.MID, UUID: m.UUID}
			if p.MailAdminReceipts == nil {
				p.MailAdminReceipts = map[string]MailAdminReceipt{}
			}
			p.MailAdminReceipts[req.Receipt] = receipt
			return nil
		})
		if err != nil {
			if businessError {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			} else {
				log.Printf("管理邮件事务失败：%v", err)
				http.Error(w, "发件事务失败，未发放", http.StatusInternalServerError)
			}
			return
		}
		if !replayed {
			s.publishMailbox(oid, p)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !replayed {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"状态": "成功", "重复请求": replayed, "mail_uuid": receipt.UUID, "mid": receipt.MID})
	})
}

var errMailReceiptConflict = errors.New("同一发件收据内容冲突")
var errMailReceiptFull = errors.New("管理发件收据已达容量，需归档后继续")
