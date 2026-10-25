package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"reflect"
	"testing"

	"hs-server/internal/game"
)

func TestPostgresAssistRefreshProjectedPeerReadOnlyAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := []string{}
	for _, name := range []string{"助战刷新请求者", "助战刷新提供者"} {
		identity, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, hex.EncodeToString(identity.Avatars[0].OID))
	}
	_, err := s.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		p := &v[ids[1]].Progress
		p.AssistCardUUID = p.Cards[0].UUID
		spare := p.Cards[0]
		spare.UUID = "00112233445566778899aacc"
		p.Cards = append(p.Cards, spare)
		p.Runes = map[string]game.Rune{"00112233445566778899aad1": {UUID: "00112233445566778899aad1", CardUUID: p.AssistCardUUID}, "00112233445566778899aad2": {UUID: "00112233445566778899aad2", CardUUID: spare.UUID}}
		p.SelectedHeadID = 4401
		p.Social.Blacklist = map[string]game.SocialFriend{ids[0]: {}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	peerOID, _ := hex.DecodeString(ids[1])
	_, err = s.pool.Exec(ctx, `UPDATE avatar_progress SET state=state || '{"非助战保留":{"数量":9223372036854775000}}'::jsonb WHERE avatar_oid=$1`, peerOID)
	if err != nil {
		t.Fatal(err)
	}
	full := pgHumanRead(t, s, peerOID)
	snapshot := func(id string) ([]byte, int64) {
		t.Helper()
		oid, _ := hex.DecodeString(id)
		var raw []byte
		var revision int64
		if err := s.pool.QueryRow(ctx, `SELECT state,revision FROM avatar_progress WHERE avatar_oid=$1`, oid).Scan(&raw, &revision); err != nil {
			t.Fatal(err)
		}
		return raw, revision
	}
	beforePeer, peerRevision := snapshot(ids[1])
	candidates, err := s.AssistCandidates(ctx, 1, 1000)
	want := game.AssistSummaryProgress(full.Progress)
	want.Runes = nil
	if err != nil || len(candidates) != 1 || !reflect.DeepEqual(game.CloneProgress(candidates[0].Progress), game.CloneProgress(want)) {
		t.Fatal("资格预选字段不符、带出无关契印或漏掉双向黑名单", err)
	}
	if _, err = s.AssistCandidates(ctx, 0, 1000); err == nil {
		t.Fatal("资格预选允许跨服无过滤")
	}
	rows, err := s.UpdateAssistRefresh(ctx, ids[0], ids, func(v map[string]*game.Avatar) error {
		if !reflect.DeepEqual(game.CloneProgress(v[ids[1]].Progress), game.CloneProgress(game.AssistSummaryProgress(full.Progress))) {
			t.Fatal("锁内助战投影与完整存档授权字段不一致")
		}
		v[ids[0]].Progress.Social.Revision++
		return nil
	})
	if err != nil || len(rows) != 1 || hex.EncodeToString(rows[0].OID) != ids[0] || len(rows[0].Progress.Cards) == 0 {
		t.Fatal("刷新输出必须仅包含请求者完整进度", err)
	}
	afterPeer, afterRevision := snapshot(ids[1])
	if !bytes.Equal(beforePeer, afterPeer) || peerRevision != afterRevision {
		t.Fatal("提供者资产、未知字段或版本被刷新重写")
	}
	beforeSelf, selfRevision := snapshot(ids[0])
	_, err = s.UpdateAssistRefresh(ctx, ids[0], ids, func(v map[string]*game.Avatar) error {
		v[ids[0]].Progress.Social.Revision++
		v[ids[1]].Progress.AssistCardUUID = ""
		return nil
	})
	if err == nil {
		t.Fatal("提供者只读投影允许修改")
	}
	afterSelf, afterSelfRevision := snapshot(ids[0])
	if !bytes.Equal(beforeSelf, afterSelf) || selfRevision != afterSelfRevision {
		t.Fatal("提供者修改拒绝未回滚请求者")
	}
	if _, err = s.UpdateAssistRefresh(ctx, ids[0], ids[1:], func(v map[string]*game.Avatar) error { return nil }); err == nil {
		t.Fatal("缺少请求者仍执行事务")
	}
}
