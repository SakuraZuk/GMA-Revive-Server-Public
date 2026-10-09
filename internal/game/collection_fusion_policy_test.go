package game

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestApprovedFurnitureFusionPoolsEveryCandidateCanActuallyBeGranted(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/data/gameplay-rules.env")
	if err != nil {
		t.Fatal(err)
	}
	var value string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "HS_FURNITURE_FUSION_POOLS='") {
			value = strings.TrimSuffix(strings.TrimPrefix(line, "HS_FURNITURE_FUSION_POOLS='"), "'")
		}
	}
	t.Setenv("HS_FURNITURE_FUSION_POOLS", value)
	pools, err := collectionFusionPools()
	if err != nil || len(pools) != 8 {
		t.Fatal("正式批准文件缺少八组候选池", err)
	}
	now := time.Unix(1800000000, 0)
	base := NewProgress(60, now)
	if err = ensureCollection(&base, now); err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for key, rows := range pools {
		for _, row := range rows {
			if row[1] != 1 {
				t.Fatal("批准的组内等概率发生漂移", key, row)
			}
			id := int(row[0])
			if seen[id] {
				continue
			}
			seen[id] = true
			p := CloneProgress(base)
			changes := map[int]int64{}
			cards := []string{}
			if err = grantNativeItem(&p, id, 1, p.AvatarLevel, now, changes, &cards, 0); err != nil {
				t.Fatal("批准候选仍无法发放", id, err)
			}
			if len(changes) == 0 && len(cards) == 0 {
				t.Fatal("批准候选返回空奖励", id)
			}
		}
	}
	// 各档位使用真实配置选择；不会把未知选择器当作原服权重。
	for tier, rule := range androidCraft.Fusion {
		if _, err = collectionFusionDraw(base, tier, rule); err != nil {
			t.Fatal("正式候选池与原生档位不兼容", tier, err)
		}
	}
	t.Logf("八组正式池已验证，实际逐项发放%d个不同候选", len(seen))
}
