package game

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestSyncPvpRejectsMissingBridgeAndWrongUUIDWithoutChangingAssets(t *testing.T) {
	p := NewProgress(1, time.Unix(100, 0))
	_, _ = prepareSyncPvpMatch(&p, "bridge-gate", time.Unix(100, 0))
	for _, battle := range []*BattleSession{nil, {UUID: "other", BridgeReady: true, BridgeStarted: true, Finished: true, Outcome: "win"}, {UUID: p.SyncPvpMatch.BattleUUID, Finished: true, Outcome: "win"}} {
		p.Battle = battle
		before, _ := json.Marshal(p)
		if _, err := settleSyncPvpResult(&p, 0, time.Unix(101, 0)); err == nil {
			t.Fatal("未通过观察桥的结果被接受")
		}
		after, _ := json.Marshal(p)
		if !bytes.Equal(before, after) {
			t.Fatal("拒绝结算仍修改积分或账本")
		}
	}
}

func TestSelectSyncPVPRobotIsDeterministicAndInDivision(t *testing.T) {
	for _, score := range []int{1000, 1200, 1400, 1600, 1800, 2400} {
		first, rule, err := selectSyncPVPRobot(score, "battle-seed-1")
		if err != nil {
			t.Fatalf("score=%d: %v", score, err)
		}
		second, _, err := selectSyncPVPRobot(score, "battle-seed-1")
		if err != nil || first.ID != second.ID {
			t.Fatalf("同种子未保持稳定: %#v %#v %v", first, second, err)
		}
		found := false
		for _, group := range first.GroupIDs {
			found = found || group == rule.RobotGroupID
		}
		if !found || len(first.SyncPvpCards) == 0 {
			t.Fatalf("机器人不属于当前 Android 分段: score=%d robot=%#v rule=%#v", score, first, rule)
		}
	}
}

func TestSelectSyncPVPRobotRejectsMissingDivision(t *testing.T) {
	if _, _, err := selectSyncPVPRobot(-1, "seed"); err == nil {
		t.Fatal("无效分数应拒绝")
	}
}

func TestPrepareSyncPvpMatchPersistsDeterministicAI(t *testing.T) {
	p := NewProgress(1, time.Unix(100, 0))
	first, err := prepareSyncPvpMatch(&p, "account-1", time.Unix(100, 0))
	if err != nil || first == nil || first.Status != syncPvpMatchReady {
		t.Fatalf("AI匹配创建失败: %#v %v", first, err)
	}
	second, err := prepareSyncPvpMatch(&p, "different-seed", time.Unix(101, 0))
	if err != nil || second.BattleUUID != first.BattleUUID || second.Robot.ID != first.Robot.ID {
		t.Fatalf("重试替换了已锁定对手: %#v %#v %v", first, second, err)
	}
}

func TestSettleSyncPvpResultIsIdempotentAndValidatesDelta(t *testing.T) {
	p := NewProgress(1, time.Unix(100, 0))
	if _, err := prepareSyncPvpMatch(&p, "account-1", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	enemy := p.SyncPvpMatch.RobotScore
	rule, _ := syncPvpScoreRuleFor(p.SyncPvpScore)
	expected := syncPvpRound(float64(rule.K) * (1 - 1/(1+math.Pow(10, float64(enemy-p.SyncPvpScore)/400))))
	p.Battle = &BattleSession{UUID: p.SyncPvpMatch.BattleUUID, BridgeReady: true, BridgeStarted: true, Finished: true, Outcome: "win"}
	result, err := settleSyncPvpResult(&p, expected, time.Unix(100, 0))
	if err != nil || result.DeltaScore != expected {
		t.Fatalf("首次结算失败: %#v %v", result, err)
	}
	score := p.SyncPvpScore
	replay, err := settleSyncPvpResult(&p, expected, time.Unix(101, 0))
	if err != nil || replay != result || p.SyncPvpScore != score {
		t.Fatalf("重复结算非幂等: %#v %#v", replay, err)
	}
}

func TestSettleSyncPvpRejectsForgedDelta(t *testing.T) {
	p := NewProgress(1, time.Unix(100, 0))
	if _, err := prepareSyncPvpMatch(&p, "account-1", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	p.Battle = &BattleSession{UUID: p.SyncPvpMatch.BattleUUID, BridgeReady: true, BridgeStarted: true, Finished: true, Outcome: "win"}
	if _, err := settleSyncPvpResult(&p, 9999, time.Unix(100, 0)); err == nil {
		t.Fatal("伪造积分变化应拒绝")
	}
}
