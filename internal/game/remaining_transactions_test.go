package game

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestTalentResetOnePointNoMaterialRefundAndPrerequisiteRollback(t *testing.T) {
	s, c, store, uuid := cardSkillTalentWorld(t, 1601)
	ctx := context.Background()
	for level := 0; level < 3; level++ {
		pushes, err := s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 0, 0, level))
		if err != nil || talentResultCode(t, pushes) != 0 {
			t.Fatal("升级前置失败", level, err)
		}
	}
	pushes, err := s.Handle(ctx, c, "upgrade_talent_node", socialArgs(uuid, 0, 1, 0))
	if err != nil || talentResultCode(t, pushes) != 0 {
		t.Fatal("升级后置失败", err)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	pushes, err = s.Handle(ctx, c, "reset_talent_node", socialArgs(uuid, 0, 0, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(pushes) == 0 || pushes[0].Method != "on_reset_talent_node" || pushes[0].Args[0] == RetSuccess {
		t.Fatal("破坏已升级后置关系仍重置成功", pushes)
	}
	stored, _ := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, stored) {
		t.Fatal("重置拒绝没有整事务回滚")
	}
	pushes, err = s.Handle(ctx, c, "reset_talent_node", socialArgs(uuid, 0, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range pushes {
		if item.Method == "on_reset_talent_node" {
			found = len(item.Args) == 4 && item.Args[0] == RetSuccess && item.Args[3] == 1
		}
	}
	if !found {
		t.Fatal("缺少原生重置成功回调", pushes)
	}
	after := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if after.Cards[0].TalentTree[0][1].Level != 0 || !reflect.DeepEqual(before.Materials, after.Materials) || !reflect.DeepEqual(before.Achievements, after.Achievements) {
		t.Fatal("重置必须仅退一潜质点，不退材料也不倒扣成就")
	}
	if _, err = s.Handle(ctx, c, "reset_talent_node", socialArgs(uuid, 0, 1, 1)); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if !reflect.DeepEqual(after, stored) {
		t.Fatal("重复旧等级请求改变进度")
	}
	if _, err = s.Handle(ctx, c, "reset_talent_node", socialArgs(uuid, 0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	if c.SelectedAvatarUnsafe().Progress.Cards[0].TalentTree[0][0].Level != 2 {
		t.Fatal("三级重置必须仅降到二级")
	}
}

func TestMailRandomRewardsFrozenAtIssueAndReload(t *testing.T) {
	ctx := context.Background()
	store := NewFixtureAccounts(nil)
	s := New(store, nil)
	c, _ := newBattleConnection(t, ctx, store, s)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	ids := []int{}
	for id, material := range androidShop.Materials {
		if material.Type == 7 && material.Sub == 1 {
			bonus := androidShop.Bonuses[material.Target]
			if len(bonus.RandomItems)+len(bonus.RandomLibs)+len(bonus.RandomRunes) > 0 {
				ids = append(ids, id)
			}
		}
	}
	sort.Ints(ids)
	chosen := 0
	for _, id := range ids {
		if err := s.IssueMail(ctx, c, Mail{MID: 601, Title: "随机礼盒冻结", Attachments: map[int]int64{id: 1, 12: 7}}); err == nil {
			chosen = id
			break
		}
	}
	if chosen == 0 {
		t.Fatal("本版没有可签发的随机礼盒")
	}
	p := c.SelectedAvatarUnsafe().Progress
	m := p.ShortMailInfo[601]
	if !m.RewardsFrozen || m.SourceAttachments[chosen] != 1 || m.Attachments[chosen] != 0 || !reflect.DeepEqual(before.Materials, p.Materials) || !reflect.DeepEqual(before.Cards, p.Cards) || !reflect.DeepEqual(before.Runes, p.Runes) {
		t.Fatal("签发没有冻结结果或提前发奖", chosen, m)
	}
	raw, _ := json.Marshal(p)
	var restored Progress
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	frozen := restored.ShortMailInfo[601]
	if !reflect.DeepEqual(m, frozen) {
		t.Fatal("随机结果JSON重载改变")
	}
	if err := claimMail(&restored, &frozen, s.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	claimed := CloneProgress(restored)
	if frozen.State != MailFinal || !reflect.DeepEqual(frozen.Attachments, m.Attachments) || !reflect.DeepEqual(frozen.Cards, m.Cards) || !reflect.DeepEqual(frozen.Runes, m.Runes) {
		t.Fatal("领取没有使用原冻结附件")
	}
	if err := claimMail(&restored, &frozen, s.Now().Unix()); err == nil || !reflect.DeepEqual(claimed, restored) {
		t.Fatal("重复领取没有零变动拒绝")
	}
	if err := s.IssueMail(ctx, c, Mail{MID: 602, Title: "伪造冻结收据", RewardsFrozen: true, Attachments: map[int]int64{12: 100}}); err == nil {
		t.Fatal("调用方可以伪造奖励冻结收据")
	}
}
