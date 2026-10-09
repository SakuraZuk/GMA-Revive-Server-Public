package game

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestMailNativeWireExpiryAndAtomicBatch(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	for _, m := range []Mail{
		{MID: 1, Title: "100%奖励", Content: "正文", Sender: "测试", Attachments: map[int]int64{12: 3}, ExpiresAt: now.Unix() + 1},
		{MID: 2, Title: "另一封", Attachments: map[int]int64{13: 1}},
	} {
		if err := svc.IssueMail(ctx, c, m); err != nil {
			t.Fatal(err)
		}
	}
	p := c.SelectedAvatarUnsafe().Progress
	mail := p.ShortMailInfo[1]
	raw, _ := json.Marshal(mail.oid())
	pushes, err := svc.Handle(ctx, c, "query_mail_content", []json.RawMessage{raw})
	if err != nil || len(pushes) != 1 || pushes[0].Method != "on_query_mail_content" {
		t.Fatal(pushes, err)
	}
	info := pushes[0].Args[1].(map[string]any)
	if info["_id"] != mail.oid() || info["sender_name"] != "测试" || info["inner_title"].(map[string]any)["template"] != "%s" {
		t.Fatal("邮件原生字段错误", info)
	}
	rows := mailProperties(p.ShortMailInfo, now.Unix())
	if len(rows) != 2 || rows[0].Value.(map[string]any)["state"] != 1 {
		t.Fatal("邮件登录投影错误", rows)
	}
	if err := svc.updateProgress(ctx, c, func(p *Progress) error {
		p.Materials[13] = Material{ID: 13, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := c.SelectedAvatarUnsafe().Progress.Materials[12].Count
	if _, err := svc.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage(`1`)}); err == nil {
		t.Fatal("批量溢出应失败")
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[12].Count != before || p.ShortMailInfo[1].State != MailUnread || p.ShortMailInfo[2].State != MailUnread {
		t.Fatal("批量失败未整体回滚")
	}
	now = now.Add(time.Second)
	if _, err := svc.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`2`), raw}); err == nil {
		t.Fatal("过期边界不能领取")
	}
	if _, err := svc.Handle(ctx, c, "delete_mail", []json.RawMessage{json.RawMessage(`3`), raw}); err != nil {
		t.Fatal("过期邮件应能删除", err)
	}
}

func TestMailBatchSynchronizesMaterialsAndPreservesReceipt(t *testing.T) {
	ctx := context.Background()
	svc := New(NewFixtureAccounts(nil), nil)
	c, _ := newBattleConnection(t, ctx, svc.Accounts.(*FixtureAccounts), svc)
	before := c.SelectedAvatarUnsafe().Progress.Materials[12].Count
	if err := svc.IssueMail(ctx, c, Mail{MID: 7, Title: "多附件", Attachments: map[int]int64{12: 2, 13: 4}}); err != nil {
		t.Fatal(err)
	}
	pushes, err := svc.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage(`1`)})
	if err != nil || len(pushes) < 3 || pushes[1].Args[0].([]any)[0] != "material_mgr" {
		t.Fatal("附件领取缺少资产推送", pushes, err)
	}
	if _, err := svc.Handle(ctx, c, "receive_all_attachments", []json.RawMessage{json.RawMessage(`2`)}); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[12].Count != before+2 || p.ShortMailInfo[7].State != MailFinal || len(p.ShortMailInfo[7].Attachments) != 2 {
		t.Fatal("领取幂等或审计附件丢失")
	}
}
