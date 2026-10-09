package dbstore

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

// 使用真实Store/Handle与SQL冷读验证获批准的永久商品，不把本机SKIP算实库通过。
func TestPostgresApprovedPermanentGoodsPurchaseColdReloadAndRejectZeroWrite(t *testing.T) {
	store := testStore(t)
	t.Setenv("HS_SERVER_OPEN_TIME", "")
	ctx := context.Background()
	now := time.Unix(1800000000+3650*86400, 0)
	info := game.ClientInfo{Account: "永久商品实库验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	_, err := store.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.AvatarLevel = 60
		p.Materials[20001] = game.Material{ID: 20001, Count: 10000, Total: 10000}
		p.Materials[10] = game.Material{ID: 10, Count: 10000, Total: 10000}
		p.UnlockSystems["card_summon"] = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1071011, 1071012, 1071013, 1071014, 2010010} {
		pushes, err := svc.Handle(ctx, c, "buy_commodity", []json.RawMessage{json.RawMessage(fmt.Sprint(id)), json.RawMessage(`1`), json.RawMessage(`0`)})
		if err != nil {
			t.Fatal("永久商品购买失败", id, err)
		}
		ok := false
		for _, response := range pushes {
			if response.Method == "on_buy_commodity" && response.Args[0] == game.RetSuccess {
				ok = true
			}
		}
		if !ok {
			t.Fatal("永久商品没有成功回包", id, pushes)
		}
	}
	loaded, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := loaded.Avatars[0].Progress
	if p.Materials[20001].Count != 8800 || p.Materials[10].Count != 9976 {
		t.Fatal("永久商品原价/折扣没有持久", p.Materials[20001], p.Materials[10])
	}
	for _, id := range []int{1071011, 1071012, 1071013, 1071014, 2010010} {
		if p.CommodityDetails[id].Total != 1 {
			t.Fatal("永久商品购买次数未持久", id)
		}
	}
	type snapshot struct {
		Avatar, State []byte
		Revision      int64
		Updated       time.Time
	}
	read := func() snapshot {
		var v snapshot
		if err := store.pool.QueryRow(ctx, `SELECT to_jsonb(a),p.state,p.revision,p.updated_at FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1`, av.OID).Scan(&v.Avatar, &v.State, &v.Revision, &v.Updated); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := read()
	pushes, err := svc.Handle(ctx, c, "buy_commodity", []json.RawMessage{json.RawMessage(`2010010`), json.RawMessage(`1`), json.RawMessage(`0`)})
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, response := range pushes {
		if response.Method == "on_buy_commodity" && response.Args[0] == 11004 {
			ok = true
		}
	}
	if !ok || !reflect.DeepEqual(before, read()) {
		t.Fatal("永久开放绕过每日限购，或拒绝后SQL发生写入", pushes)
	}
}
