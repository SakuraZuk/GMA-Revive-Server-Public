package game

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMailLifecycleAndIdempotentReceive(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	service := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, service)
	before, _ := c.SelectedAvatar()
	base := before.Progress.Materials[12].Count
	if err := service.IssueMail(ctx, c, Mail{MID: 9, Title: "测试", Attachments: map[int]int64{12: 3}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "delete_mail", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`9`)}); err == nil {
		t.Fatal("未领取邮件不应允许删除")
	}
	if _, err := service.Handle(ctx, c, "read_mail", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`9`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`9`)}); err != nil {
		t.Fatal(err)
	}
	av, _ := c.SelectedAvatar()
	if av.Progress.Materials[12].Count != base+3 || av.Progress.ShortMailInfo[9].State != MailFinal {
		t.Fatalf("邮件领取状态错误: %#v", av.Progress)
	}
	if _, err := service.Handle(ctx, c, "receive_attachment", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`9`)}); err == nil {
		t.Fatal("重复领取应拒绝")
	}
	if _, err := service.Handle(ctx, c, "delete_mail", []json.RawMessage{json.RawMessage(`5`), json.RawMessage(`9`)}); err != nil {
		t.Fatal(err)
	}
	av, _ = c.SelectedAvatar()
	if _, ok := av.Progress.ShortMailInfo[9]; ok {
		t.Fatal("邮件未删除")
	}
}
