package game

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlayerAdminConcurrentReceiptRollbackAndOnlineFreshness(t *testing.T) {
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, context.Background(), a, s)
	s.attachPlayer(c)
	defer s.Detach(c)
	token := strings.Repeat("a", 32)
	handler := AdminHandler(s, token)
	req := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "资产001", "operator": "测试员", "reason": "并发发放", "operation": "material_add", "material_id": 12, "amount": 5}
	call := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/admin/player/update", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body, _ := json.Marshal(req)
	before, _ := a.AdminPlayer(context.Background(), av.OID)
	var wg sync.WaitGroup
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- call(body).Code }()
	}
	wg.Wait()
	close(results)
	created := 0
	for code := range results {
		if code == 201 {
			created++
		} else if code != 200 {
			t.Fatal("并发管理失败", code)
		}
	}
	if created != 1 {
		t.Fatal("并发收据重复发放", created)
	}
	_, err := a.UpdateProgress(context.Background(), av.OID, func(p *Progress) error { m := p.Materials[12]; m.Count--; p.Materials[12] = m; return nil })
	if err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Tick(context.Background(), c)
	if err != nil || len(pushes) == 0 {
		t.Fatal("在线刷新失败", err)
	}
	current, _ := c.SelectedAvatar()
	if current.Progress.Materials[12].Count != before.Progress.Materials[12].Count+4 {
		t.Fatal("管理旧快照覆盖之后的消费")
	}
	req["amount"] = 6
	body, _ = json.Marshal(req)
	if call(body).Code != 422 {
		t.Fatal("收据冲突未拒绝")
	}
	_, err = a.UpdateProgress(context.Background(), av.OID, func(p *Progress) error {
		p.Materials[12] = Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	req["receipt"] = "资产002"
	body, _ = json.Marshal(req)
	if call(body).Code != 422 {
		t.Fatal("溢出未拒绝")
	}
	currentAv, _ := a.AdminPlayer(context.Background(), av.OID)
	if len(currentAv.Progress.PlayerAdminReceipts) != 1 || currentAv.Progress.Materials[12].Count != math.MaxInt64 {
		t.Fatal("拒绝仍写入审计或资产")
	}
	props := currentAv.InitialProperties(currentAv.Account)
	if _, ok := props["server_player_admin_receipts"]; ok {
		t.Fatal("管理审计泄漏到原生属性")
	}
}

func TestPlayerAdminRecommendationReceiptAndRejectedOffer(t *testing.T) {
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	s.Now = func() time.Time { return time.Unix(1800000000, 0) }
	_, av := newBattleConnection(t, context.Background(), a, s)
	token := strings.Repeat("r", 32)
	h := AdminHandler(s, token)
	req := playerAdminRequest{AvatarOID: hex.EncodeToString(av.OID), Receipt: "推荐报价001", Operator: "运营员", Reason: "按原表折扣区间发行", Operation: "recommendation_issue", Offer: &ShopRecommendation{ID: 801001, Commodity: 8010001, Price: 648, ShowDuration: 120, Expired: s.Now().Add(time.Hour).Unix()}}
	call := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/admin/player/update", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(); w.Code != 200 {
		t.Fatal("同一报价收据不能安全重试", w.Code, w.Body.String())
	}
	current, err := a.AdminPlayer(context.Background(), av.OID)
	if err != nil {
		t.Fatal(err)
	}
	receipt := current.Progress.PlayerAdminReceipts[req.Receipt]
	if receipt.Request == nil || receipt.Request.Offer == nil || receipt.Request.Offer.Price != 648 || current.Progress.CommodityDetails[8010001].Recommendation.Issued != s.Now().Unix() {
		t.Fatal("发行报价未真实保存或审计缺少报价详情")
	}
	req.Receipt = "推荐报价002"
	req.Offer.Price = 1
	if w := call(); w.Code != 422 {
		t.Fatal("低于Android折扣区间仍发行", w.Code)
	}
	current, _ = a.AdminPlayer(context.Background(), av.OID)
	if len(current.Progress.PlayerAdminReceipts) != 1 || current.Progress.CommodityDetails[8010001].Recommendation.Price != 648 {
		t.Fatal("拒绝报价留下审计或覆盖合法报价")
	}
}

func TestPlayerAdminProfileCardAndPrivateQuery(t *testing.T) {
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	_, av := newBattleConnection(t, context.Background(), a, s)
	token := strings.Repeat("a", 32)
	h := AdminHandler(s, token)
	call := func(method, path string, value any, auth bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(value)
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if call("GET", "/admin/players", nil, false).Code != 401 {
		t.Fatal("玩家查询不需要令牌")
	}
	id := hex.EncodeToString(av.OID)
	req := map[string]any{"avatar_oid": id, "receipt": "资料001", "operator": "测试员", "reason": "合法资料", "operation": "profile", "nickname": "测试馆主", "level": 10, "gender": 2}
	if w := call("POST", "/admin/player/update", req, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	req = map[string]any{"avatar_oid": id, "receipt": "幻书001", "operator": "测试员", "reason": "真实模板", "operation": "card_grant", "card_id": 1101, "count": 2}
	if w := call("POST", "/admin/player/update", req, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	view := call("GET", "/admin/players/"+id, nil, true)
	if view.Code != 200 || strings.Contains(view.Body.String(), `"account"`) || strings.Contains(view.Body.String(), "password") {
		t.Fatal("查询失败或暴露凭据", view.Body.String())
	}
	got, _ := a.AdminPlayer(context.Background(), av.OID)
	if got.Info.Level != 10 || got.Info.Nickname != "测试馆主" || got.Gender != 2 || len(got.Progress.Cards) != 3 {
		t.Fatal("资料或幻书未持久")
	}
	if got.Progress.Cards[1].UUID == got.Progress.Cards[2].UUID || got.Progress.Cards[1].Level != 1 {
		t.Fatal("幻书实例不独立或非初始状态")
	}
}
