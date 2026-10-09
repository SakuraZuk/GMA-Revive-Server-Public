package game

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCollectionSocialVisitLikeAtomicDayAndColdReload(t *testing.T) {
	ctx := context.Background()
	s, cs, store := socialTestWorld(t)
	now := time.Date(2026, 10, 8, 23, 59, 59, 0, time.FixedZone("UTC+8", 8*3600))
	s.Now = func() time.Time { return now }
	a, b := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	if _, e := store.UpdateSocial(ctx, []string{a, b}, func(rows map[string]*Avatar) error {
		for _, id := range []string{a, b} {
			p := &rows[id].Progress
			p.UnlockSystems["real_house_visit"] = 1
			if e := ensureCollection(p, now); e != nil {
				return e
			}
			p.Collection.Rooms[1] = newCollectionRoom(1, now)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	call := func(c *Connection, method string, args ...any) []Push {
		t.Helper()
		pushes, e := s.Handle(ctx, c, method, socialArgs(args...))
		if e != nil {
			t.Fatal(e)
		}
		return pushes
	}
	code := func(pushes []Push) int {
		for _, item := range pushes {
			if item.Method == "on_like_friend_house" {
				return item.Args[1].(int)
			}
			if item.Method == "on_query_friend_dormitory" || item.Method == "on_end_visiting_house" {
				return item.Args[0].(int)
			}
		}
		t.Fatal("缺少收藏室原生回包", pushes)
		return -1
	}
	beforeA, beforeB := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	initial := call(cs[0], "like_friend_house", b)
	if code(initial) != androidCollection.Errors["RET_HOUSE_VISITING_NO_THIS_FRIEND"] {
		t.Fatal("没有真实访问便可点赞", initial)
	}
	if !reflect.DeepEqual(beforeA, socialSaved(t, store, cs[0])) || !reflect.DeepEqual(beforeB, socialSaved(t, store, cs[1])) {
		t.Fatal("未授权点赞写入存档")
	}
	pushes := call(cs[0], "query_friend_dormitory", b, 1)
	if code(pushes) != RetSuccess {
		t.Fatal("陌生人允许访问未放行")
	}
	var total map[string]any
	for _, item := range pushes {
		if item.Method == "on_query_friend_dormitory" {
			total = item.Args[2].(map[string]any)
		}
	}
	if total["__custom_type"] != "restroom.house_total_info" || total["player_info"].(map[string]any)["__custom_type"] != "avatar_info.avatar_info" {
		t.Fatal("原生访问自定义类型丢失")
	}
	if code(call(cs[0], "like_friend_house", b)) != RetSuccess {
		t.Fatal("真实访问点赞失败")
	}
	own, peer := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if !own.Progress.Collection.LikedOwners[b] || peer.Progress.Collection.ReceivedLikes != 1 {
		t.Fatal("双方点赞事务未保存")
	}
	for _, id := range []int{210504, 210505, 210506} {
		if own.Progress.Achievements[id].Targets[210556] != 1 {
			t.Fatal("真实给赞未接成就", id)
		}
	}
	for _, id := range []int{210507, 210508, 210509} {
		if peer.Progress.Achievements[id].Targets[210555] != 1 {
			t.Fatal("真实获赞未接成就", id)
		}
	}
	if code(call(cs[0], "like_friend_house", b)) != androidCollection.Errors["RET_HOUSE_LIKE_ALREADY"] || !reflect.DeepEqual(own, socialSaved(t, store, cs[0])) || !reflect.DeepEqual(peer, socialSaved(t, store, cs[1])) {
		t.Fatal("同日重复点赞不幂等")
	}
	now = now.Add(time.Second)
	if code(call(cs[0], "like_friend_house", b)) != RetSuccess {
		t.Fatal("跨UTC+8日界没有恢复点赞资格")
	}
	peer = socialSaved(t, store, cs[1])
	if peer.Progress.Collection.ReceivedLikes != 2 || peer.Progress.Collection.DailyReceivedLikes[20261008] != 1 || peer.Progress.Collection.DailyReceivedLikes[20261009] != 1 {
		t.Fatal("跨日获赞累计或每日计数错误")
	}
	raw, e := json.Marshal(peer.Progress)
	if e != nil {
		t.Fatal(e)
	}
	var cold Progress
	if e = json.Unmarshal(raw, &cold); e != nil || !reflect.DeepEqual(cold.Collection, peer.Progress.Collection) {
		t.Fatal("冷读取收藏室访问状态丢失", e)
	}
	call(cs[0], "end_visiting_house")
	if socialSaved(t, store, cs[0]).Progress.Collection.VisitOwner != "" {
		t.Fatal("结束访问未清授权")
	}
}

func TestCollectionSocialLikeCapacityBlockAndWholePairRollback(t *testing.T) {
	ctx := context.Background()
	s, cs, store := socialTestWorld(t)
	now := s.Now()
	a, b := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
	set := func(fn func(*Avatar, *Avatar)) {
		t.Helper()
		_, e := store.UpdateSocial(ctx, []string{a, b}, func(rows map[string]*Avatar) error { fn(rows[a], rows[b]); return nil })
		if e != nil {
			t.Fatal(e)
		}
	}
	set(func(self, peer *Avatar) {
		for _, v := range []*Avatar{self, peer} {
			v.Progress.UnlockSystems["real_house_visit"] = 1
			if e := ensureCollection(&v.Progress, now); e != nil {
				t.Fatal(e)
			}
			v.Progress.Collection.Rooms[1] = newCollectionRoom(1, now)
		}
		self.Progress.Collection.VisitOwner = b
		self.Progress.Collection.VisitTime = float64(now.Unix())
		self.Progress.Collection.LikeDay = collectionDay(now)
		peer.Progress.Collection.ReceivedLikes = math.MaxInt64
	})
	beforeA, beforeB := socialSaved(t, store, cs[0]), socialSaved(t, store, cs[1])
	if _, e := s.Handle(ctx, cs[0], "like_friend_house", socialArgs(b)); e == nil {
		t.Fatal("获赞溢出未拒绝")
	}
	if !reflect.DeepEqual(beforeA, socialSaved(t, store, cs[0])) || !reflect.DeepEqual(beforeB, socialSaved(t, store, cs[1])) {
		t.Fatal("溢出留下单边点赞或成就")
	}
	set(func(self, peer *Avatar) {
		peer.Progress.Collection.ReceivedLikes = 0
		peer.Progress.Collection.AllowVisitors = false
	})
	pushes, e := s.Handle(ctx, cs[0], "query_friend_dormitory", socialArgs(b, 1))
	ret := -1
	for _, item := range pushes {
		if item.Method == "on_query_friend_dormitory" {
			ret = item.Args[0].(int)
		}
	}
	if e != nil || ret != androidCollection.Errors["RET_HOUSE_VISIT_NOT_FRIEND"] {
		t.Fatal("陌生人绕过访问隐私", e, pushes)
	}
	set(func(self, peer *Avatar) {
		peer.Progress.Collection.AllowVisitors = true
		self.Progress.Collection.LikedOwners = map[string]bool{}
		for i := 0; i < 99; i++ {
			self.Progress.Collection.LikedOwners[fmt.Sprintf("%024x", i+100)] = true
		}
	})
	pushes, e = s.Handle(ctx, cs[0], "like_friend_house", socialArgs(b))
	ret = -1
	for _, item := range pushes {
		if item.Method == "on_like_friend_house" {
			ret = item.Args[1].(int)
		}
	}
	if e != nil || ret != androidCollection.Errors["RET_HOUSE_LIKE_DAILY_LIMIT"] {
		t.Fatal("每日99次上限未拒绝", e)
	}
}
