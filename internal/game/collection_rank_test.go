package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestCollectionRankHistoryDuplicatesFilterAndNativeReply(t *testing.T) {
	ids := CollectionRankCardIDs()
	if len(ids) != 82 {
		t.Fatal("本版统计卡目录变化，须复核原生规则")
	}
	p := Progress{Cards: []Card{{CardID: ids[0]}, {CardID: ids[0]}, {CardID: ids[1]}, {CardID: 4}}, ObtainedCardIDs: map[int]int64{ids[0]: 1, ids[2]: 2, 4: 3, 999999: 4}}
	before := CloneProgress(p)
	if n := CollectionRankCount(p); n != 3 {
		t.Fatal("重复、素材、未知编号或已取得历史计数错误", n)
	}
	if !reflect.DeepEqual(before, p) {
		t.Fatal("收集榜查询改写原进度")
	}
	records := map[string]FixtureAccount{}
	for i := 0; i < 105; i++ {
		name := fmt.Sprintf("收集榜%d", i)
		oid := make([]byte, 12)
		oid[11] = byte(i + 1)
		records[name] = FixtureAccount{Password: "pw", Avatars: []Avatar{{OID: oid, UID: int64(i + 1), Account: name, Hostnum: 1, Info: DefaultAvatarInfo(name), Progress: p}}}
	}
	accounts := NewFixtureAccounts(records)
	svc := New(accounts, nil)
	c := NewConnection()
	c.phase = Playing
	c.hostnum = 1
	c.identity = Identity{Account: "收集榜104", Avatars: records["收集榜104"].Avatars}
	rows, own, err := accounts.ActivityRanking(context.Background(), 2, 0, c.SelectedAvatarUnsafe().OID, 1, 1)
	if err != nil || len(rows) != 1 || own != 105 {
		t.Fatal("页面上限截断了个人实际名次", err, len(rows), own)
	}
	out, err := svc.Handle(context.Background(), c, "query_rank_list", []json.RawMessage{json.RawMessage("2"), json.RawMessage("0")})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range out {
		if v.Method == "on_query_rank_list" {
			found = true
			if v.Args[0] != "2&0" || len(v.Args[1].([]any)) != 100 {
				t.Fatal("原生榜号或100条上限错误", v.Args[0])
			}
		}
	}
	if !found {
		t.Fatal("实际Handle缺原生排行回包")
	}
	if _, err = svc.Handle(context.Background(), c, "query_rank_list", []json.RawMessage{json.RawMessage("2"), json.RawMessage("1")}); err == nil {
		t.Fatal("收集榜错误接受子榜")
	}
}
