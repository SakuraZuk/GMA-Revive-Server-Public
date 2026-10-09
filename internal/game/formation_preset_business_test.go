package game

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func presetTestSetup(t *testing.T) (*FixtureAccounts, *Service, *Connection, Avatar, []Card) {
	t.Helper()
	accounts, service, conn, avatar, cards := layoutTestSetup(t)
	_, err := accounts.UpdateProgress(context.Background(), avatar.OID, func(p *Progress) error {
		p.UnlockSystems["support"] = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return accounts, service, conn, avatar, cards
}

func presetRaw(t *testing.T, fighting, support []any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"fighting_cards":  fighting,
		"support_cards":   support,
		"storyline_cards": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFormationPresetStableSlotsAndRPCs(t *testing.T) {
	accounts, service, conn, avatar, cards := presetTestSetup(t)
	ctx := context.Background()
	before, err := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if err != nil || len(before.PresetIDs) != presetRecordNum {
		t.Fatalf("预设槽位初始化失败: %v %#v", err, before.PresetIDs)
	}
	ids := append([]string{}, before.PresetIDs...)
	pushes, err := service.Handle(ctx, conn, "cover_preset_record", []json.RawMessage{
		json.RawMessage(`7`), json.RawMessage(`"` + ids[0] + `"`), presetRaw(t, []any{cards[0].UUID}, []any{cards[1].UUID}),
	})
	if err != nil || len(pushes) == 0 {
		t.Fatalf("覆盖预设失败: %v %#v", err, pushes)
	}
	after, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if after.PresetIDs[0] != ids[0] || after.PresetCardsRecord[ids[0]].Cards.Fighting[0] != cards[0].UUID {
		t.Fatal("预设内容或稳定槽位丢失")
	}
	if _, err := service.Handle(ctx, conn, "update_preset_name", []json.RawMessage{json.RawMessage(`8`), json.RawMessage(`"` + ids[0] + `"`), json.RawMessage(`"主力"`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(ctx, conn, "update_preset_index", []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`"` + ids[0] + `"`), json.RawMessage(`"` + ids[1] + `"`), json.RawMessage(`true`)}); err != nil {
		t.Fatal(err)
	}
	latest, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if latest.PresetIDs[1] != ids[0] || latest.PresetCardsRecord[ids[0]].Name != "主力" {
		t.Fatal("预设改名或排序失败")
	}
	if _, err := service.Handle(ctx, conn, "del_preset_record", []json.RawMessage{json.RawMessage(`10`), json.RawMessage(`"` + ids[0] + `"`)}); err != nil {
		t.Fatal(err)
	}
	deleted, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if _, ok := deleted.PresetCardsRecord[ids[0]]; ok || len(deleted.PresetIDs) != presetRecordNum {
		t.Fatal("删除预设错误地删除了槽位")
	}
	props := (Avatar{Progress: deleted}).InitialProperties("测试")
	if _, ok := props["preset_cards_record"]; !ok || !reflect.DeepEqual(props["preset_ids"].([]any), func() []any {
		out := make([]any, len(deleted.PresetIDs))
		for i, id := range deleted.PresetIDs {
			out[i] = ObjectID(id)
		}
		return out
	}()) {
		t.Fatal("预设登录下发未保留ObjectID槽位")
	}
}

func TestFormationPresetRejectsDuplicateCardAndInvalidNameAtomically(t *testing.T) {
	accounts, service, conn, avatar, cards := presetTestSetup(t)
	ctx := context.Background()
	p, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	bad := presetRaw(t, []any{cards[0].UUID}, []any{cards[0].UUID})
	if _, err := service.Handle(ctx, conn, "cover_preset_record", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"` + p.PresetIDs[0] + `"`), bad}); err == nil {
		t.Fatal("重复同名卡被接受")
	}
	before, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if _, err := service.Handle(ctx, conn, "update_preset_name", []json.RawMessage{json.RawMessage(`2`), json.RawMessage(`"` + p.PresetIDs[0] + `"`), json.RawMessage(`"一二三四五六七八"`)}); err == nil {
		t.Fatal("超长预设名被接受")
	}
	after, _ := accounts.UpdateProgress(ctx, avatar.OID, func(*Progress) error { return nil })
	if !reflect.DeepEqual(before, after) {
		t.Fatal("非法预设请求部分提交")
	}
}

func TestFormationPresetSyncAndActivityUseAndroidCatalog(t *testing.T) {
	accounts, service, conn, avatar, _ := presetTestSetup(t)
	ctx := context.Background()
	var activityCards []Card
	positions := []*int{}
	for i := 0; i < 6; i++ {
		positions = append(positions, nil)
		if duty, ok := androidFormationPresetCatalog.MiniDuty[itoa(i+1)]; ok {
			positions[i] = duty.LimitPosition
		}
	}
	for cardID, rule := range androidFormationPresetCatalog.Cards {
		if len(activityCards) == 6 || rule.Disable != 0 || containsInt(rule.Forbid, androidBattleForbidden) {
			continue
		}
		for index, required := range positions {
			if required != nil && (rule.Position == nil || *required != *rule.Position) {
				continue
			}
			id, _ := strconv.Atoi(cardID)
			duplicate := false
			for _, existing := range activityCards {
				if existing.CardID == id {
					duplicate = true
				}
			}
			if !duplicate && index == len(activityCards) {
				activityCards = append(activityCards, newCard(id, 1, time.Now()))
				break
			}
		}
	}
	if len(activityCards) != 6 {
		t.Skip("Android 1.0.128 当前表没有足够的六职责可战卡")
	}
	if _, err := accounts.UpdateProgress(ctx, avatar.OID, func(p *Progress) error { p.Cards = activityCards; return nil }); err != nil {
		t.Fatal(err)
	}
	empty := presetRaw(t, []any{}, []any{})
	if _, err := service.Handle(ctx, conn, "set_sync_pvp_preset_record", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`0`), empty}); err != nil {
		t.Fatal("同步PVP空预设应允许：", err)
	}
	values := make([]any, len(activityCards))
	for i, card := range activityCards {
		values[i] = card.UUID
	}
	activityRaw, _ := json.Marshal(map[string]any{"fighting_cards": values, "support_cards": []any{}, "storyline_cards": []any{}})
	if _, err := service.Handle(ctx, conn, "activity_set_preset_record", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`"mini_team"`), activityRaw}); err != nil {
		t.Fatal("活动六职责预设失败：", err)
	}
}
