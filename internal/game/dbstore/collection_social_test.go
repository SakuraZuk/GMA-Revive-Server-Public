package dbstore

import (
	"context"
	"encoding/hex"
	"hs-server/internal/game"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestPostgresCollectionVisitLikesPairRollbackAndColdRetry(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 23, 59, 59, 0, time.FixedZone("UTC+8", 8*3600))
	infoA := game.ClientInfo{Account: "收藏室点赞甲", Password: "pw", Hostnum: 1}
	infoB := game.ClientInfo{Account: "收藏室点赞乙", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, infoA, &now)
	svcB, cB, avB := repairPlayer(t, store, infoB, &now)
	defer svc.Detach(c)
	defer svcB.Detach(cB)
	owner := game.ObjectID(hex.EncodeToString(avB.OID))
	for _, oid := range [][]byte{av.OID, avB.OID} {
		if _, e := store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
			p.UnlockSystems["real_house_visit"] = 1
			p.Materials[809] = game.Material{ID: 809, Count: 1, Total: 1}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := svc.Handle(ctx, c, "unlock_dormitory", pgSocialArgs(809)); e != nil {
		t.Fatal(e)
	}
	if _, e := svcB.Handle(ctx, cB, "unlock_dormitory", pgSocialArgs(809)); e != nil {
		t.Fatal(e)
	}
	read := func(oid []byte) game.Progress {
		t.Helper()
		value, e := New(store.pool).AdminPlayer(ctx, oid)
		if e != nil {
			t.Fatal(e)
		}
		return game.CloneProgress(value.Progress)
	}
	call := func(method string, args ...any) []game.Push {
		t.Helper()
		out, e := svc.Handle(ctx, c, method, pgSocialArgs(args...))
		if e != nil {
			t.Fatal(method, e)
		}
		return out
	}
	out := call("query_friend_dormitory", owner, 1)
	reply, _ := pgNativeReply(t, out, "on_query_friend_dormitory")
	if reply.Args[0] != game.RetSuccess {
		t.Fatal("实库授权访问失败", reply)
	}
	out = call("like_friend_house", owner)
	reply, at := pgNativeReply(t, out, "on_like_friend_house")
	if reply.Args[1] != game.RetSuccess {
		t.Fatal("实库点赞失败", reply)
	}
	pgNativePropertyBefore(t, out, at, "house_likes_map")
	pA, pB := read(av.OID), read(avB.OID)
	if pB.Collection.ReceivedLikes != 1 || pA.Achievements[210504].Targets[210556] != 1 || pB.Achievements[210507].Targets[210555] != 1 {
		t.Fatal("实库双角色获赞成就未同时保存")
	}
	out = call("like_friend_house", owner)
	reply, _ = pgNativeReply(t, out, "on_like_friend_house")
	if reply.Args[1] == game.RetSuccess || !reflect.DeepEqual(pA, read(av.OID)) || !reflect.DeepEqual(pB, read(avB.OID)) {
		t.Fatal("实库重复点赞写入状态")
	}
	// 新 Service 与新 Store 冷读，既有访问授权和同日去重不能因重建而丢失。
	cold := pgSocialService(t, New(store.pool))
	cold.Now = func() time.Time { return now }
	coldC := pgSocialLogin(t, cold, infoA, av.OID)
	_, e := cold.Handle(ctx, coldC, "like_friend_house", pgSocialArgs(owner))
	if e != nil || read(avB.OID).Collection.ReceivedLikes != 1 {
		t.Fatal("冷重建重复发赞", e)
	}
	now = now.Add(time.Second)
	out, e = cold.Handle(ctx, coldC, "like_friend_house", pgSocialArgs(owner))
	if e != nil {
		t.Fatal(e)
	}
	reply, _ = pgNativeReply(t, out, "on_like_friend_house")
	if reply.Args[1] != game.RetSuccess || read(avB.OID).Collection.ReceivedLikes != 2 {
		t.Fatal("真实库UTC+8跨日未恢复资格")
	}
	if _, e = store.UpdateProgress(ctx, avB.OID, func(p *game.Progress) error { p.Collection.ReceivedLikes = math.MaxInt64; return nil }); e != nil {
		t.Fatal(e)
	}
	now = now.Add(24 * time.Hour)
	pA, pB = read(av.OID), read(avB.OID)
	if _, e = cold.Handle(ctx, coldC, "like_friend_house", pgSocialArgs(owner)); e == nil {
		t.Fatal("实库获赞溢出未拒绝")
	}
	if !reflect.DeepEqual(pA, read(av.OID)) || !reflect.DeepEqual(pB, read(avB.OID)) {
		t.Fatal("实库失败保留单边日界或成就改动")
	}
}
