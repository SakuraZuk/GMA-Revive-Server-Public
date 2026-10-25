package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNianEmptyHourlySnapshotNoWriteAndEntryExit(t *testing.T) {
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, shanghaiZone)
	oid, _ := SocialOID("111111111111111111111111")
	av := &Avatar{OID: oid, Hostnum: 1, Progress: NewProgress(20, now)}
	rows := map[string]*Avatar{hexOf(oid): av}
	freezeNianHourRanking(rows, now)
	before, _ := json.Marshal(av.Progress)
	freezeNianHourRanking(rows, now.Add(time.Hour))
	after, _ := json.Marshal(av.Progress)
	if string(before) != string(after) {
		t.Fatal("未参赛空榜仅因整点变化改写存档")
	}
	// 小时内新成绩须等下一整点，零伤害仍是真实参赛。
	av.Progress.Activities.Nian = map[int]ActivityProgress{20200001: {Ranked: true, RankAt: now.Add(70 * time.Minute).Unix(), RankDamage: 0}}
	freezeNianHourRanking(rows, now.Add(90*time.Minute))
	if _, ok := FrozenNianTotalRank(*av); ok {
		t.Fatal("当前小时新成绩提前进冻结榜")
	}
	freezeNianHourRanking(rows, now.Add(2*time.Hour))
	if row, ok := FrozenNianTotalRank(*av); !ok || row.Rank != 1 {
		t.Fatal("真实零分参赛未在下一整点入榜")
	}
	av.Progress.Activities.Nian = nil
	freezeNianHourRanking(rows, now.Add(3*time.Hour))
	if _, ok := FrozenNianTotalRank(*av); ok {
		t.Fatal("旧榜成绩消失后未清掉历史名次")
	}
	before, _ = json.Marshal(av.Progress)
	freezeNianHourRanking(rows, now.Add(4*time.Hour))
	after, _ = json.Marshal(av.Progress)
	if string(before) != string(after) {
		t.Fatal("已清空的榜仍逐小时改写")
	}
}
