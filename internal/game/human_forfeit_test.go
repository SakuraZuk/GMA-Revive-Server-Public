package game

import (
	"context"
	"reflect"
	"testing"
)

func TestHumanForfeitActualRPCDurablePeerAndRepeat(t *testing.T) {
	s, cs, store, r := humanTestPair(t, true)
	ctx := context.Background()
	loser, winner := cs[0], cs[1]
	loserID, winnerID := hexOf(selectedOID(loser)), hexOf(selectedOID(winner))
	before := socialSaved(t, store, winner).Progress.SyncPvpScore
	// 传输关闭只移除在线连接，不提交弃权。
	s.Detach(loser)
	if got := socialSaved(t, store, loser).Progress.Social.HumanRoom; got.Status != "fighting" || got.Winner != "" {
		t.Fatal("网络断开被误判为弃权", got)
	}
	reconnected := NewConnection()
	reconnected.identity, reconnected.hostnum, reconnected.phase = loser.identity, loser.hostnum, Playing
	reconnected.humanDeliveryUUID, reconnected.humanDeliverySequence = loser.humanDeliveryUUID, loser.humanDeliverySequence
	loser, cs[0] = reconnected, reconnected
	pushes, err := s.Handle(ctx, loser, "exit_battle", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertForfeit := func(c *Connection, out []Push) {
		t.Helper()
		if len(out) < 3 || out[0].Args[0] != "set_winner_eid_list" || out[1].Args[0] != "real_battle_end" {
			t.Fatal("没有先按原生顺序结束双方场景", out)
		}
		for _, p := range out {
			if p.Method == "battle_result" {
				extra := p.Args[3].(map[string]any)
				if extra["both_results_agree"] != false || extra["reason"] != "player_forfeit" {
					t.Fatal("主动弃权被冒充为双方原生结果一致", extra)
				}
			}
		}
	}
	assertForfeit(loser, pushes)
	other := New(store, nil)
	other.Now = s.Now
	assertForfeit(winner, other.flushHumanPvp(winner))
	for _, c := range cs {
		p := socialSaved(t, store, c).Progress
		if p.Social.HumanRoom.UUID != r.UUID || p.Social.HumanRoom.Winner != winnerID || p.Social.HumanRoom.Status != "settled" || len(p.SyncPvpRecords) != 1 || len(p.SyncPvpSettlements) != 1 {
			t.Fatal("双方收据与唯一胜方未同事务保存", p.Social.HumanRoom)
		}
		if (p.Battle.Outcome == "loss") != (hexOf(selectedOID(c)) == loserID) {
			t.Fatal("弃权方胜负颠倒")
		}
	}
	if socialSaved(t, store, winner).Progress.SyncPvpScore <= before {
		t.Fatal("胜方积分没有实际增加")
	}
	for _, method := range []string{"exit_battle", "leave_battle", "quit_battle"} {
		for _, c := range cs {
			old := socialSaved(t, store, c).Progress
			if _, err := s.Handle(ctx, c, method, nil); err != nil {
				t.Fatal(err)
			}
			next := socialSaved(t, store, c).Progress
			if old.SyncPvpScore != next.SyncPvpScore || !reflect.DeepEqual(old.SyncPvpRecords, next.SyncPvpRecords) || !reflect.DeepEqual(old.SyncPvpSettlements, next.SyncPvpSettlements) || next.Social.HumanRoom.Winner != winnerID || len(old.Social.HumanRoom.Deliveries) != len(next.Social.HumanRoom.Deliveries) {
				t.Fatal("重复退出翻转胜方或重复发奖")
			}
		}
	}
}

func TestHumanForfeitDoesNotOverridePendingNativeResultOrLoadingFailure(t *testing.T) {
	s, cs, store, r := humanTestPair(t, true)
	ctx := context.Background()
	humanTestEvent(t, s, cs[0], 3, "human_result", map[string]any{"action": 1, "winner_eids": []string{r.IDs[0]}, "units": humanTestUnits(r), "command_index": 0})
	before := socialSaved(t, store, cs[1]).Progress
	if _, err := s.Handle(ctx, cs[1], "exit_battle", nil); err == nil {
		t.Fatal("退出覆盖了已经收到的原生末态")
	}
	if !reflect.DeepEqual(before, socialSaved(t, store, cs[1]).Progress) {
		t.Fatal("拒绝退出仍写入双方进度")
	}
	s, cs, store, r = humanTestPair(t, true)
	_, err := store.UpdateSocial(ctx, r.IDs, func(v map[string]*Avatar) error {
		for _, id := range r.IDs {
			v[id].Progress.Social.HumanRoom.Status = "loading"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before = socialSaved(t, store, cs[0]).Progress
	if _, err = s.Handle(ctx, cs[0], "exit_battle", nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		p := socialSaved(t, store, c).Progress
		if p.Social.HumanRoom.Status != "aborted" || p.Battle.Outcome != "" || len(p.SyncPvpRecords) != 0 || p.SyncPvpScore != before.SyncPvpScore {
			t.Fatal("加载取消被错误结算成胜负")
		}
	}
}
