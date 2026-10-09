package game

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCollectionNativeUnlockDeploymentAndPersistence(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, av := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		ensureCollection(p, now)
		p.Materials[809] = Material{ID: 809, Count: 1, Total: 1}
		p.Materials[810] = Material{ID: 810, Count: 1, Total: 1}
		p.Materials[801] = Material{ID: 801, Count: 1, Total: 1}
		p.Materials[19] = Material{ID: 19, Count: 100000, Total: 100000}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if len(c.SelectedAvatarUnsafe().Progress.Collection.Rooms) != 0 {
		t.Fatal("初始化凭空解锁收藏室")
	}
	rpc := func(method string, values ...any) []Push {
		t.Helper()
		args := []json.RawMessage{}
		for _, v := range values {
			b, _ := json.Marshal(v)
			args = append(args, b)
		}
		out, e := s.collectionRPC(ctx, c, method, args)
		if e != nil {
			t.Fatal(method, e)
		}
		return out
	}
	out := rpc("unlock_dormitory", 809)
	if out[len(out)-1].Method != "on_unlock_dormitory" || out[len(out)-1].Args[0] != RetSuccess {
		t.Fatal("原生房间解锁回包错误")
	}
	out = rpc("unlock_facility", 801)
	if out[len(out)-1].Method != "on_unlock_facility" || len(out[len(out)-1].Args) != 3 {
		t.Fatal("原生设施解锁回包错误")
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	out = rpc("unlock_dormitory", 809)
	if out[len(out)-1].Args[0] != androidCollection.Errors["RET_HOUSE_ALREADY_UNLOCK"] || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("重复解锁扣料或改存档")
	}
	rpc("dormitory_change_house_card", 1, [][]int{{4401, 1, androidCardAppearances[4401].DefaultDress}})
	p := c.SelectedAvatarUnsafe().Progress
	if p.Collection.Rooms[1].Slots[1] != 4401 || p.Achievements[210101].Targets[210101] != 1 {
		t.Fatal("入住或成就未真实推进")
	}
	now = now.Add(10 * time.Second)
	out = rpc("receive_restroom_exp", 9, 1)
	args := out[len(out)-1].Args[1].([]any)
	if args[1] != int64(2) {
		t.Fatal("原生5秒好感度间隔错误", args)
	}
	stored, e := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if e != nil || !reflect.DeepEqual(stored.Avatars[0].Progress, c.SelectedAvatarUnsafe().Progress) {
		t.Fatal("收藏室状态未持久", e)
	}
	_ = av
}

func TestCollectionFurnitureHandbookAtomicRewardAndLayout(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Materials: map[int]Material{}}
	ensureCollection(&p, now)
	p.Collection.Rooms[1] = newCollectionRoom(1, now)
	if p.Collection.Furniture[11012] != 1 {
		t.Fatal("Android初始家具缺失")
	}
	ensureCollection(&p, now)
	if p.Collection.Furniture[11012] != 1 {
		t.Fatal("重复初始化重发家具")
	}
	if e := grantCollectionFurniture(&p, 11001, 1, now); e != nil {
		t.Fatal(e)
	}
	row := CollectionPlacement{1, 0, 1, 10, 10, 11001, 0}
	if e := validateCollectionLayout(p, []CollectionPlacement{row}, nil); e != nil {
		t.Fatal("合法原生家具布局失败", e)
	}
	if e := validateCollectionLayout(p, []CollectionPlacement{row, row}, nil); e == nil {
		t.Fatal("重叠家具未拒绝")
	}
	outside := row
	outside[3] = 100
	if e := validateCollectionLayout(p, []CollectionPlacement{outside}, nil); e == nil {
		t.Fatal("家具越界未拒绝")
	}
	unowned := row
	unowned[5] = 11002
	if e := validateCollectionLayout(p, []CollectionPlacement{unowned}, nil); e == nil {
		t.Fatal("布局上传凭空制造家具")
	}
	items := map[int]int64{}
	cards := []string{}
	if e := collectionClaimHandbook(&p, "receive_furniture_handbook_bonus", 11001, now, items, &cards); e != nil {
		t.Fatal(e)
	}
	if p.Collection.Handbook[11001] != 2 || p.Materials[11].Count != 5 {
		t.Fatal("家具手册真实奖励错误")
	}
	if e := collectionClaimHandbook(&p, "receive_furniture_handbook_bonus", 11001, now, items, &cards); e == nil {
		t.Fatal("重复手册领奖未拒绝")
	}
}

func TestCollectionHandbookOverflowRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	c, _ := newBattleConnection(t, ctx, a, s)
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		ensureCollection(p, now)
		if e := grantCollectionFurniture(p, 11001, 1, now); e != nil {
			return e
		}
		p.Materials[11] = Material{ID: 11, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	before := CloneProgress(c.SelectedAvatarUnsafe().Progress)
	if _, e := s.collectionRPC(ctx, c, "receive_furniture_handbook_bonus", []json.RawMessage{json.RawMessage(`11001`)}); e == nil {
		t.Fatal("奖励余额溢出未拒绝")
	}
	stored, e := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if e != nil || !reflect.DeepEqual(before, stored.Avatars[0].Progress) {
		t.Fatal("奖励失败未整事务回滚", e)
	}
}

func TestProfileTargetUnlockAndRealStatistics(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Cards: []Card{{CardID: 4401}, {CardID: 4401}, {CardID: 4403}}, ClearedDungeons: []int{102, 103, 21110001}}
	ensureProfileCosmetics(&p, AvatarInfo{}, now)
	for _, id := range []int{2, 4, 5} {
		if _, owned := p.OwnedHeadBox[id]; !owned {
			t.Fatal("原生登录目标头像未解锁", id)
		}
	}
	if _, owned := p.OwnedHeadBox[1053]; owned {
		t.Fatal("没有资格的兑换头像框被解锁")
	}
	props := profileStatisticsProperties(p)
	if props["cards_count"] != 1 || props["last_main_chapter_dungeon_id"] != 103 {
		t.Fatal("真实唯一幻书或主线统计错误", props)
	}
	p.Cards = nil
	if profileStatisticsProperties(p)["cards_count"] != 0 {
		t.Fatal("已分解幻书仍误计为当前持有")
	}
}
