package game

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAdminRuneGrantAuthConcurrentIdempotencyAndAudit(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	_, av := newBattleConnection(t, context.Background(), accounts, s)
	token := strings.Repeat("x", 32)
	handler := AdminHandler(s, token)
	request := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "测试发放001", "operator": "管理员", "reason": "契印功能验收", "spec": RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 11}}
	body, _ := json.Marshal(request)
	call := func(remote, auth string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/admin/runes/grant", bytes.NewReader(body))
		r.RemoteAddr = remote
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := call("127.0.0.1:12345", "", body); got.Code != http.StatusUnauthorized {
		t.Fatal("没有令牌仍可管理发放")
	}
	if got := call("192.0.2.1:12345", "Bearer "+token, body); got.Code != http.StatusForbidden {
		t.Fatal("非回环仍可管理发放")
	}
	if got := call("127.0.0.1:12345", "Bearer "+token, append(append([]byte{}, body...), []byte("{}")...)); got.Code != http.StatusBadRequest {
		t.Fatal("多余 JSON 被接受")
	}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); responses <- call("127.0.0.1:12345", "Bearer "+token, body) }()
	}
	wg.Wait()
	close(responses)
	created := 0
	uuid := ""
	for response := range responses {
		if response.Code == http.StatusCreated {
			created++
		} else if response.Code != http.StatusOK {
			t.Fatalf("发放失败: %d %s", response.Code, response.Body.String())
		}
		var result struct {
			Rune struct {
				UUID string `json:"uuid"`
			} `json:"rune"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if uuid == "" {
			uuid = result.Rune.UUID
		}
		if uuid != result.Rune.UUID {
			t.Fatal("并发重试生成了多个 UUID")
		}
	}
	if created != 1 {
		t.Fatal("并发重试不是恰好一次发放")
	}
	p, err := accounts.UpdateProgress(context.Background(), av.OID, func(*Progress) error { return nil })
	if err != nil || len(p.Runes) != 1 || len(p.RuneGrantReceipts) != 1 || p.RuneAdminAudit["admin:测试发放001"].Reason != "契印功能验收" {
		t.Fatal("奖励、凭据和审计未一起保存", err)
	}
	request["reason"] = "另一原因"
	conflict, _ := json.Marshal(request)
	if got := call("127.0.0.1:12345", "Bearer "+token, conflict); got.Code != http.StatusUnprocessableEntity {
		t.Fatal("同凭据可修改审计内容")
	}
	s.Accounts = runeFailedProgressStore{accounts}
	if got := call("127.0.0.1:12345", "Bearer "+token, body); got.Code != http.StatusInternalServerError || strings.Contains(got.Body.String(), "故障注入") {
		t.Fatal("存储故障未正确拒绝或泄露内部异常")
	}
}
