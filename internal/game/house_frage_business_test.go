package game

import (
	"context"
	"encoding/json"
	"fmt"
	"hs-server/internal/mobileproto"
	"reflect"
	"testing"
	"time"
)

func TestActivityHouseRPCPersistenceDailyAndMoveRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		p.Collection.Facilities[6] = CollectionFacility{ID: 6, Level: 1}
		p.Materials[401] = Material{ID: 401}
		p.Materials[402] = Material{ID: 402, Count: 1, Total: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rpc := func(method string, values ...any) []Push {
		t.Helper()
		args := []json.RawMessage{}
		for _, v := range values {
			raw, _ := json.Marshal(v)
			args = append(args, raw)
		}
		out, err := s.Handle(ctx, c, method, args)
		if err != nil {
			t.Fatal(method, err)
		}
		return out
	}
	out := rpc("get_house_board_game_reward")
	found := false
	for _, push := range out {
		if push.Method == "on_get_house_board_game_reward" && fmt.Sprint(push.Args[0]) == "0" {
			found = true
		}
	}
	if !found {
		t.Fatal("每日骰子回包签名", out)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[401].Count != 2 || p.Collection.Facilities[6].BoardReward {
		t.Fatal("每日骰子未按设施效果入账", p.Materials[401])
	}
	rpc("get_house_board_game_reward")
	if c.SelectedAvatarUnsafe().Progress.Materials[401].Count != 2 {
		t.Fatal("每日骰子重复入账")
	}
	rpc("enter_house_frage", 1)
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if len(before.Activities.House.Sites) != 63 || before.Activities.House.Pos != 1 {
		t.Fatal("原生棋盘缺失")
	}
	rpc("enter_house_frage", 2)
	if !reflect.DeepEqual(before.Activities.House, c.SelectedAvatarUnsafe().Progress.Activities.House) {
		t.Fatal("未结束棋盘重入重新生成")
	}
	rpc("house_frage_ctrl_move", 3, 1)
	p = c.SelectedAvatarUnsafe().Progress
	if p.Activities.House.Pos != 2 || p.Materials[402].Count != 0 {
		t.Fatal("遥控骰子未按确定路径扣费", p.Activities.House.Pos)
	}
	for _, aid := range []int{209001, 209002, 209003} {
		if p.Achievements[aid].Targets[1008] != 1 {
			t.Fatal("真实遥控骰子消耗未累计原生无参成就", aid, p.Achievements[aid])
		}
	}
	before = CloneProgress(p)
	rpc("house_frage_ctrl_move", 4, 6)
	if !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("骰子不足失败未整事务回滚")
	}
	rpc("house_frage_move", 5, 2)
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[401].Count != 1 {
		t.Fatal("双骰模式必须只扣一个普通骰子")
	}
	for _, aid := range []int{209001, 209002, 209003} {
		expect := int64(2)
		if aid == 209001 {
			expect = 1 // 原生第一档目标为1，达标后按目标封顶。
		}
		if p.Achievements[aid].Targets[1008] != expect {
			t.Fatal("双骰步数被误算为两次骰子消耗", aid, p.Achievements[aid])
		}
	}
	stored, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || !reflect.DeepEqual(p.Activities.House, stored.Avatars[0].Progress.Activities.House) {
		t.Fatal("宿舍棋盘未持久化", err)
	}
	now = now.Add(24 * time.Hour)
	rpc("get_house_board_game_reward")
	if c.SelectedAvatarUnsafe().Progress.Materials[401].Count != 3 {
		t.Fatal("次日骰子资格未恢复")
	}
}

func TestActivityCthulhuNativeAttributeClamp(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Materials: map[int]Material{53: {ID: 53, Count: 90, Total: 110}, 54: {ID: 54, Count: 12, Total: 15}}}
	box, err := grantCthulhuBonus(&p, []int{20631001}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Materials[53].Count != 100 || p.Materials[53].Total != 120 || box["materials"].(map[int]int64)[53] != 10 {
		t.Fatal("理智封顶与实际奖励差额错误", box, p.Materials[53])
	}
	box, err = grantCthulhuBonus(&p, []int{20631001}, now)
	if err != nil || len(box["materials"].(map[int]int64)) != 0 || p.Materials[53].Total != 120 {
		t.Fatal("已满理智虚增获得总量", err, box)
	}
	copy := CloneProgress(p)
	if _, err = grantNativeBonus(&copy, 20631001, 1, 1, now); err == nil {
		t.Fatal("专用属性策略不应绕过普通库存容量保护")
	}
}

// 移动回调的 rewards 由 Android 原生 GUI 调用 iteritems，动画候选在客户端生成。
// 用真实金币格核对回包、资产差额、失败空字典与持久化，防止再次把动画候选入账。
func TestActivityHouseMoveNativeRewardDictionary(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, a, s)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		p.Collection.Facilities[6] = CollectionFacility{ID: 6, Level: 1}
		w := ensureHouseFrage(p)
		w.Pos = 2
		w.Sites = map[int]*HouseFrageSite{2: {ID: 2}, 3: {ID: 3}}
		p.Materials[402] = Material{ID: 402, Count: 1, Total: 1}
		p.Materials[18] = Material{ID: 18, Count: 7, Total: 7}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	move := func() []any {
		t.Helper()
		out, err := s.Handle(ctx, c, "house_frage_ctrl_move", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`1`)})
		if err != nil {
			t.Fatal(err)
		}
		return out[len(out)-1].Args[1].([]any)
	}
	reply := move()
	rewards, ok := reply[3].(mobileproto.Map)
	if !ok || len(rewards) != 1 || rewards[0].Key != int64(18) || rewards[0].Value != int64(10) {
		t.Fatal("原生移动奖励未发实际材料字典", reply)
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[18].Count != 17 || p.Materials[18].Total != 17 || p.Materials[402].Count != 0 {
		t.Fatal("动画候选污染实际奖励或扣骰子错误", p.Materials)
	}
	before := CloneProgress(p)
	reply = move()
	rewards, ok = reply[3].(mobileproto.Map)
	if !ok || len(rewards) != 0 || reply[0] != int64(0) || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("拒绝移动未发空奖励字典或未回滚", reply)
	}
	stored, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || stored.Avatars[0].Progress.Materials[18].Count != 17 {
		t.Fatal("原生字典奖励未持久化", err)
	}
}
