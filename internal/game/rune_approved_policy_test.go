package game

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func approvedGameplayValue(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../deploy/data/gameplay-rules.env")
	if err != nil {
		t.Fatal(err)
	}
	prefix := name + "='"
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), "'")
		}
	}
	t.Fatal("正式规则缺少已批准变量", name)
	return ""
}

func TestApprovedRunePolicyAllPoolsAndEveryCandidateActuallyGenerate(t *testing.T) {
	value := approvedGameplayValue(t, "HS_RUNE_FALLBACK_POOLS")
	t.Setenv("HS_RUNE_FALLBACK_POOLS", value)
	var pools map[int][][2]int
	if err := json.Unmarshal([]byte(value), &pools); err != nil || len(pools) != 17 {
		t.Fatal("正式17池配置无效", err)
	}
	now := time.Unix(1800000000, 0)
	for dropID, rows := range pools {
		if len(rows) != 18 {
			t.Fatal("批准套装数发生漂移", dropID)
		}
		drop := androidShop.RuneDrops[dropID]
		selected, err := nativeRuneSuitRows(dropID, drop.Suits)
		if err != nil || len(selected) != len(rows) {
			t.Fatal("正式规则没有进入实际奖励函数", dropID, err)
		}
		p := Progress{}
		for _, row := range rows {
			if row[1] != 1 {
				t.Fatal("正式组内等概率发生漂移", dropID, row)
			}
			for position, weight := range drop.Positions {
				if weight == 0 {
					continue
				}
				for star, probability := range drop.Stars {
					if probability == 0 {
						continue
					}
					// 独立背包检查每个有原生概率的位置/星级，避免只测一次随机结果。
					p = Progress{}
					if _, err := GrantRuneWithMarks(&p, RuneSpec{Suit: row[0], Position: position + 1, Star: star + 1, Level: 1}, drop.Marks, now); err != nil {
						t.Fatal("批准候选在原生部位/星级不能生成", dropID, row[0], position+1, star+1, err)
					}
				}
			}
		}
		p = Progress{}
		if err := grantShopRandomRunes(&p, []json.RawMessage{json.RawMessage(fmt.Sprintf("[%d,[2],1]", dropID))}, 1, now); err != nil || len(p.Runes) != 2 {
			t.Fatal("实际随机奖励未保留数量", dropID, err)
		}
		allowed := map[int]bool{}
		for _, row := range rows {
			allowed[row[0]] = true
		}
		for _, rune := range p.Runes {
			if !allowed[rune.Suit] || drop.Positions[rune.Position-1] == 0 || drop.Stars[rune.Star-1] == 0 {
				t.Fatal("实际掉落偏离正式套装或原生星级/部位", dropID, rune)
			}
		}
	}
}
