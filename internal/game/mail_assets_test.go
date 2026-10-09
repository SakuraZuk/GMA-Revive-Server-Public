package game

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMailCardsAtomicClaimAndNativeContent(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	card := newCard(4401, 1, s.Now())
	m := Mail{MID: 201, Title: "幻书附件", Cards: []Card{card}, Attachments: map[int]int64{12: 2}}
	if err := s.IssueMail(ctx, c, m); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	oldCount := len(p.Cards)
	oldMaterial := p.Materials[12].Count
	content := mailContent(p.ShortMailInfo[201], ObjectID("000000000000000000000001"))
	if content["has_attachemnt"] != true || content["card_dict"] == nil {
		t.Fatal("附件原生投影缺失")
	}
	wireCard := content["card_dict"].(map[string]any)[card.UUID].(map[string]any)
	if wireCard["grade"] != 0 {
		t.Fatal("原生初始附件品阶必须为0", wireCard)
	}
	raw, _ := json.Marshal(p.ShortMailInfo[201].oid())
	if _, err := s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage("1"), raw}); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if len(p.Cards) != oldCount+1 || p.Materials[12].Count != oldMaterial+2 || p.ShortMailInfo[201].State != MailFinal {
		t.Fatal("卡与材料未一起领取")
	}
	if _, claimed := findCard(&p, card.UUID); claimed == nil || claimed.Grade != 0 {
		t.Fatal("原生初始附件领取后品阶错误")
	}
	if _, err := s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage("2"), raw}); err == nil {
		t.Fatal("重复领取被接受")
	}
	if err := s.IssueMail(ctx, c, Mail{MID: 202, Title: "重复资产", Cards: []Card{card}}); err == nil {
		t.Fatal("拥有卡UUID可再次发放")
	}
	card.UUID = newCardUUID()
	card.Dress = 9
	if err := s.IssueMail(ctx, c, Mail{MID: 203, Title: "非法外观", Cards: []Card{card}}); err == nil {
		t.Fatal("邮件绕过外观门槛")
	}
}

func TestMailNewCardGradeRejectsNonNativeTemplateAtomically(t *testing.T) {
	for _, example := range []struct {
		name  string
		grade int
	}{
		{"旧版品阶1不可新发", 1}, {"高品阶不可新发", 2}, {"负品阶不可新发", -1},
	} {
		t.Run(example.name, func(t *testing.T) {
			ctx := context.Background()
			accounts := NewFixtureAccounts(nil)
			s := New(accounts, nil)
			c, _ := newBattleConnection(t, ctx, accounts, s)
			original, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
			if err != nil {
				t.Fatal(err)
			}
			before := CloneProgress(original.Avatars[0].Progress)
			card := newCard(4401, 1, s.Now())
			card.Grade = example.grade
			if err := s.IssueMail(ctx, c, Mail{MID: 204, Title: "非法初始品阶", Cards: []Card{card}, Attachments: map[int]int64{12: 2}}); err == nil {
				t.Fatal("非原生品阶模板可签发新邮件")
			}
			stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
			if err != nil || !reflect.DeepEqual(before, CloneProgress(stored.Avatars[0].Progress)) {
				t.Fatal("非法邮件签发改变了整进度", err)
			}
		})
	}
}

func TestMailStoredGradeOneAttachmentAndInventoryPreserved(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, s)
	legacy := newCard(4401, 1, s.Now())
	legacy.Grade = 1
	ownedUUID := c.SelectedAvatarUnsafe().Progress.Cards[0].UUID
	// 模拟上版已持久的库存及待领附件，不通过现在的新签发校验造旧历史。
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.Cards[0].Grade = 1
		if p.ShortMailInfo == nil {
			p.ShortMailInfo = map[int]Mail{}
		}
		p.ShortMailInfo[205] = Mail{MID: 205, Title: "旧版待领附件", Cards: []Card{legacy}, State: MailUnread, CreatedAt: s.Now().Unix()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`205`)}); err != nil {
		t.Fatal("旧版已签发品阶1附件无法领取", err)
	}
	stored, err := accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := stored.Avatars[0].Progress
	_, owned := findCard(&p, ownedUUID)
	_, claimed := findCard(&p, legacy.UUID)
	if owned == nil || claimed == nil || owned.Grade != 1 || claimed.Grade != 1 || p.ShortMailInfo[205].Cards[0].Grade != 1 || p.ShortMailInfo[205].State != MailFinal {
		t.Fatal("旧版库存或附件品阶被覆盖")
	}
	before := CloneProgress(p)
	if _, err = s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`205`)}); err == nil {
		t.Fatal("旧版附件可以重复领取")
	}
	stored, err = accounts.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(before, CloneProgress(stored.Avatars[0].Progress)) {
		t.Fatal("旧版附件重复领取改变整进度", err)
	}
}

func TestMailboxNotificationRevisionAndDetach(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, ctx, accounts, s)
	// 测试夹具没有走BecomePlayer，显式注册。
	c.mu.Lock()
	s.attachPlayer(c)
	c.mu.Unlock()
	if err := s.IssueMail(ctx, c, Mail{MID: 301, Title: "通知", Attachments: map[int]int64{12: 1}}); err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Tick(ctx, c)
	if err != nil || len(pushes) < 2 || pushes[0].Method != "client_prop_changed" || pushes[1].Method != "notify_new_mail" || len(pushes[1].Args) != 0 {
		t.Fatal("在线通知顺序或签名错误", pushes, err)
	}
	old := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	raw, _ := json.Marshal(old.ShortMailInfo[301].oid())
	if _, err := s.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage("1"), raw}); err != nil {
		t.Fatal(err)
	}
	s.publishMailbox(av.OID, old)
	if pushes, err := s.Tick(ctx, c); err != nil {
		t.Fatal(err)
	} else {
		for _, p := range pushes {
			if p.Method == "notify_new_mail" {
				t.Fatal("旧通知触发了新邮件提示")
			}
		}
	}
	if c.SelectedAvatarUnsafe().Progress.ShortMailInfo[301].State != MailFinal {
		t.Fatal("旧快照覆盖已领状态")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.publishMailbox(av.OID, old) }()
	}
	s.Detach(c)
	wg.Wait()
	if len(s.online) != 0 || c.pendingMailbox != nil || c.phase != Closed {
		t.Fatal("断线注册表未清理")
	}
}

func TestAdminMailConcurrentReceiptAuditAndOnlineNotification(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	c.mu.Lock()
	s.attachPlayer(c)
	c.mu.Unlock()
	token := strings.Repeat("m", 32)
	h := AdminHandler(s, token)
	req := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "发件001", "operator": "验收员", "reason": "管理邮件验收", "mail": Mail{MID: 401, Title: "事务邮件", Attachments: map[int]int64{12: 3}}}
	body, _ := json.Marshal(req)
	call := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/admin/mail/issue", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:2000"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var wg sync.WaitGroup
	responses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); responses <- call(body).Code }()
	}
	wg.Wait()
	close(responses)
	created := 0
	for code := range responses {
		if code == 201 {
			created++
		} else if code != 200 {
			t.Fatal("并发发件失败", code)
		}
	}
	if created != 1 {
		t.Fatal("重复发信", created)
	}
	p, err := a.UpdateProgress(ctx, av.OID, func(*Progress) error { return nil })
	if err != nil || len(p.ShortMailInfo) != 1 || len(p.MailAdminReceipts) != 1 || p.MailAdminReceipts["发件001"].Reason != "管理邮件验收" {
		t.Fatal("邮件收据审计未一起持久", err)
	}
	pushes, err := s.Tick(ctx, c)
	if err != nil || len(pushes) < 2 || pushes[1].Method != "notify_new_mail" {
		t.Fatal("管理邮件未在线通知", err)
	}
	req["reason"] = "修改请求"
	conflict, _ := json.Marshal(req)
	if got := call(conflict); got.Code != 422 {
		t.Fatal("收据可复用其他内容", got.Code)
	}
	s.Accounts = runeFailedProgressStore{a}
	if got := call(body); got.Code != 500 {
		t.Fatal("事务失败仍发件", got.Code)
	}
}

func TestMailRuneCapacityBatchRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	r, err := GenerateRune(RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 1}, s.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.IssueMail(ctx, c, Mail{MID: 501, Title: "契印附件", Runes: map[string]Rune{r.UUID: r}, Attachments: map[int]int64{12: 2}}); err != nil {
		t.Fatal(err)
	}
	if err = s.IssueMail(ctx, c, Mail{MID: 502, Title: "冲突附件", Runes: map[string]Rune{r.UUID: r}}); err == nil {
		t.Fatal("待领契印UUID冲突被接受")
	}
	tables, _ := loadRuneTables()
	if err = s.updateProgress(ctx, c, func(p *Progress) error {
		p.Runes = map[string]Rune{}
		for i := 0; i < tables.Tables.Limits.Max; i++ {
			p.Runes[itoa(i)] = Rune{}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := c.SelectedAvatarUnsafe().Progress.Materials[12].Count
	pushes, err := s.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage("1")})
	if err != nil || pushes[len(pushes)-1].Args[1].([]any)[0] == 0 {
		t.Fatal("满契印背包未返回原生失败", err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[12].Count != before || p.ShortMailInfo[501].State != MailUnread {
		t.Fatal("容量失败未整体回滚")
	}
	if err = s.updateProgress(ctx, c, func(p *Progress) error { p.Runes = map[string]Rune{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage("2")}); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.Runes[r.UUID].UUID != r.UUID {
		t.Fatal("契印附件未领取")
	}
}

func TestAdminStaticPageBoundary(t *testing.T) {
	h := AdminHandler(New(NewFixtureAccounts(nil), nil), strings.Repeat("x", 32))
	for _, path := range []string{"/admin", "/admin/ui.js"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "127.0.0.1:9001"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("管理静态页不可用或缺安全边界", path, w.Code)
		}
		r.RemoteAddr = "192.0.2.1:9001"
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("管理静态页开放公网")
		}
	}
}
