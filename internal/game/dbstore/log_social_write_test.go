package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"hs-server/internal/game"
	"reflect"
	"testing"
)

func TestPostgresLogSocialReadOnlyPeerAndAssistFilter(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	ids := []string{}
	for _, name := range []string{"日志写入本人", "日志只读对方"} {
		v, e := store.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, hex.EncodeToString(v.Avatars[0].OID))
	}
	revisions := func() []int64 {
		t.Helper()
		v := []int64{}
		for _, id := range ids {
			oid, _ := hex.DecodeString(id)
			var n int64
			if e := store.pool.QueryRow(ctx, `SELECT revision FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&n); e != nil {
				t.Fatal(e)
			}
			v = append(v, n)
		}
		return v
	}
	before := revisions()
	_, err := store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		v[ids[0]].Progress.AssistCardUUID = v[ids[0]].Progress.Cards[0].UUID
		v[ids[0]].Progress.OwnedLeagues = map[string]*game.LeagueState{ids[0]: {ID: ids[0], Host: 1}}
		v[ids[0]].Progress.SelectedHeadID = 4401
		v[ids[0]].Progress.SyncPvpScore = 1234
		v[ids[0]].Progress.Social.Blacklist = map[string]game.SocialFriend{ids[1]: {}}
		p := &v[ids[0]].Progress
		spare := p.Cards[0]
		spare.UUID = "00112233445566778899aacc"
		p.Cards = append(p.Cards, spare)
		p.Runes = map[string]game.Rune{
			"00112233445566778899aad1": {UUID: "00112233445566778899aad1", CardUUID: p.AssistCardUUID},
			"00112233445566778899aad2": {UUID: "00112233445566778899aad2", CardUUID: spare.UUID},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after := revisions()
	if after[0] != before[0]+1 || after[1] != before[1] {
		t.Fatal("只读对手仍被重写", before, after)
	}
	rows, err := New(store.pool).SocialAvatars(ctx, game.SocialSearch{AssistOnly: true, Limit: 1000})
	if err != nil || len(rows) != 1 || hex.EncodeToString(rows[0].OID) != ids[0] {
		t.Fatal("助战候选未先过滤", err, rows)
	}
	full := rows[0].Progress
	rows, err = New(store.pool).SocialAvatars(ctx, game.SocialSearch{AssistOnly: true, AssistSummaryOnly: true, Limit: 1000})
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(game.CloneProgress(rows[0].Progress), game.CloneProgress(game.AssistSummaryProgress(full))) {
		t.Fatal("助战预读投影与完整存档资格字段不一致", err)
	}
	if len(rows[0].Progress.Cards) != 1 || len(rows[0].Progress.Runes) != 1 || len(rows[0].Progress.Materials) != 0 || len(rows[0].Progress.OwnedLeagues) != 0 {
		t.Fatal("助战投影携带无关存档")
	}
	rows, err = New(store.pool).SocialAvatars(ctx, game.SocialSearch{LeagueOwnersOnly: true, Hostnum: 1, Limit: 1000})
	if err != nil || len(rows) != 1 || hex.EncodeToString(rows[0].OID) != ids[0] {
		t.Fatal("学会主档未先过滤", err, rows)
	}
	rows, err = New(store.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: []string{ids[0]}, FriendSummaryOnly: true})
	if err != nil || len(rows) != 1 {
		t.Fatal("推荐摘要查询失败", err)
	}
	p := rows[0].Progress
	if len(p.Cards) != 0 || len(p.Materials) != 0 || p.SelectedHeadID != 4401 || p.SyncPvpScore != 1234 || len(p.Social.Blacklist) != 1 {
		t.Fatal("推荐摘要泄漏背包或丢失过滤资料")
	}
	_, err = store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		v[ids[0]].Progress.AssistCardUUID = ""
		v[ids[1]].Progress.AssistCardUUID = "无效"
		return errors.New("回滚注入")
	})
	if err == nil {
		t.Fatal("事务未失败")
	}
	end := revisions()
	if end[0] != after[0] || end[1] != after[1] {
		t.Fatal("回滚仍写入", after, end)
	}
	_, err = store.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	end = revisions()
	if end[0] != after[0] || end[1] != after[1] {
		t.Fatal("只读事务改变版本")
	}
}
