package game

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestRuneStoneNativeStagesCostReplayAndPrivacy(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, shanghaiZone)
	s.Now = func() time.Time { return now }
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		p.AvatarLevel = 60
		p.Materials[23] = Material{ID: 23, Count: 2, Total: 2}
		p.Materials[11] = Material{ID: 11, Count: 10000000, Total: 10000000}
		p.Materials[12] = Material{ID: 12, Count: 10000000, Total: 10000000}
		ok, err := grantRuneStone(p, 40007, now)
		if !ok || err != nil {
			t.Fatal("原石未入档", err)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Handle(ctx, c, "step_0_rune_stone", rawArgs(93, 40007))
	if err != nil || len(out) != 4 {
		t.Fatal("开启回调或同步缺失", err, out)
	}
	wireInfo, ok := out[3].Args[1].([]any)[2].(map[ObjectID]any)
	if !ok || len(wireInfo) != 10 {
		t.Fatal("原生回调没有ObjectId键")
	}
	p := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if p.Materials[23].Count != 1 || p.Materials[11].Count != 10000000 || p.RuneStone.OpenCount != 1 || len(p.RuneStone.Runes) != 10 {
		t.Fatal("开启未按原表优先扣材料或抽取数量错误")
	}
	ids := []string{}
	for id := range p.RuneStone.Runes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ids = ids[:3]
	for _, info := range runeStoneInfo(p.RuneStone, nil) {
		if len(info.(map[string]any)) != 2 {
			t.Fatal("开启提前泄露部位或属性")
		}
	}
	props := (Avatar{Progress: p}).InitialProperties("")["current_rune_stone"].(map[string]any)
	if _, leaked := props["runes"]; leaked {
		t.Fatal("隐藏契印下发")
	}
	if _, leaked := props["server_day"]; leaked {
		t.Fatal("服务端日界下发")
	}
	if _, err = s.Handle(ctx, c, "step_1_rune_stone", rawArgs(94, ids)); err != nil {
		t.Fatal(err)
	}
	p = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	for _, info := range runeStoneInfo(p.RuneStone, ids) {
		if len(info.(map[string]any)) != 4 {
			t.Fatal("打磨字段错误")
		}
	}
	before := CloneProgress(p)
	if _, err = s.Handle(ctx, c, "step_1_rune_stone", rawArgs(95, []string{sortedStoneIDs(p.RuneStone)[3]})); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("超量打磨未回滚")
	}
	cost := int64(0)
	for _, id := range ids {
		r := p.RuneStone.Runes[id]
		for _, entry := range runeStoneCatalog.Prices.Entries {
			if entry.Key[0] == r.Star && entry.Key[1] == r.Position {
				cost += int64(entry.Value.Materials[1])
			}
		}
	}
	if _, err = s.Handle(ctx, c, "step_2_rune_stone", rawArgs(96, ids)); err != nil {
		t.Fatal(err)
	}
	p = CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if p.Materials[12].Count != 10000000-cost || p.Materials[11].Count != 10000000 {
		t.Fatal("鉴定金币不符原表")
	}
	for _, id := range ids {
		if !reflect.DeepEqual(p.Runes[id], p.RuneStone.Runes[id]) || p.RuneStone.States[id] != 3 {
			t.Fatal("鉴定未发真实原件")
		}
	}
	before = CloneProgress(p)
	if _, err = s.Handle(ctx, c, "step_2_rune_stone", rawArgs(97, ids)); err != nil || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("鉴定重传重复扣费发奖", err)
	}
	// 原版重开参数相同也属于合法新抽取，不以参数永久去重。
	if _, err = s.Handle(ctx, c, "step_0_rune_stone", rawArgs(98, 40007)); err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[23].Count != 0 || p.RuneStone.OpenCount != 2 {
		t.Fatal("合法重开未扣费")
	}
	for _, id := range ids {
		if _, retained := p.Runes[id]; !retained {
			t.Fatal("重开删除已鉴定库存")
		}
	}
}

func sortedStoneIDs(stone *RuneStoneState) []string {
	ids := []string{}
	for id := range stone.Runes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestRuneStoneDailyExpiryCostFallbackAndFailureRollback(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	p := NewProgress(60, time.Now())
	now := time.Date(2026, 10, 13, 23, 30, 0, 0, shanghaiZone)
	if ok, err := grantRuneStone(&p, 40007, now); !ok || err != nil {
		t.Fatal(err)
	}
	if ok, err := grantRuneStone(&p, 40008, now.Add(time.Hour)); ok || err != nil || p.RuneStone.Count != 0 {
		t.Fatal("跨日仍有效的原石不能覆盖", ok, err)
	}
	if ok, err := grantRuneStone(&p, 40008, now.Add(3*time.Hour)); !ok || err != nil || p.RuneStone.Count != 1 {
		t.Fatal("跨日过期应可领取", ok, err)
	}
	if ok, err := grantRuneStone(&p, 40007, now.Add(6*time.Hour)); ok || err != nil {
		t.Fatal("同日过期不能绕过每日限制", ok, err)
	}
	now = now.Add(3 * time.Hour)
	p.Materials[23] = Material{ID: 23}
	p.Materials[11] = Material{ID: 11, Count: 10, Total: 10}
	if _, err := openRuneStone(&p, 40008, now); err != nil || p.Materials[11].Count != 0 {
		t.Fatal("开启备用材料错误", err)
	}
	before, _ := json.Marshal(p)
	if _, err := openRuneStone(&p, 40008, now); err == nil {
		t.Fatal("缺材料开启成功")
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("拒绝开启变更资产或隐藏状态")
	}
	if _, err := advanceRuneStone(&p, sortedStoneIDs(p.RuneStone)[:1], false, now.Add(3*time.Hour)); err == nil {
		t.Fatal("过期仍能打磨")
	}
	if errRuneStoneCallback(errors.New("存储故障")) == nil || errRuneStoneCallback(runeStoneReject("资格不符")) != nil {
		t.Fatal("存储故障被吞或业务拒绝伪装故障")
	}
	before, _ = json.Marshal(p)
	if ok, err := grantRuneStone(&p, 40007, now.Add(-24*time.Hour)); ok || err != nil {
		t.Fatal("旧日冻结奖励不得回拨次数或阻塞", err)
	}
	after, _ = json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("旧日冻结奖励覆盖新日状态")
	}
}

func TestEquipmentBaseLuckyScopeAndFrozenStoneReplay(t *testing.T) {
	t.Setenv("HS_RUNE_FALLBACK_POOLS", approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS"))
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, shanghaiZone)
	p := NewProgress(60, now)
	if _, err := grantNativeBonusRaw(&p, 610701, 1, 60, now); err == nil {
		t.Fatal("商店等普通调用越界启用幸运基础盒")
	}
	b := &BattleSession{UUID: "00112233445566778899aabb", DungeonID: 6107}
	trial := CloneProgress(p)
	if _, err := settleOrdinaryDungeonRewards(&trial, b, true, now); err != nil {
		t.Fatal(err)
	}
	if len(b.RewardBonusPlan) == 0 || trial.RuneStone == nil || trial.RuneStone.MaterialID != 40007 {
		t.Fatal("基础盒或原石没有冻结来源")
	}
	b.RewardGranted = false
	b.RewardPlanFrozen = true
	b.RewardPlannedAt = now.Unix()
	if _, err := settleOrdinaryDungeonRewards(&p, b, true, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Runes, trial.Runes) || !reflect.DeepEqual(p.RuneStone, trial.RuneStone) {
		t.Fatal("冻结兑现重新抽契印或变更原石日期")
	}
	before, _ := json.Marshal(p)
	if _, err := settleOrdinaryDungeonRewards(&p, b, true, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("冻结重传重复奖励")
	}
}
