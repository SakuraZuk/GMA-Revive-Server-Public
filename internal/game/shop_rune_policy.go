package game

import (
	"encoding/json"
	"errors"
	"os"
)

// Android未携带全零套装权重的服务端候选池；只有明确的本服运营配置能提供候选。
// 例：HS_RUNE_FALLBACK_POOLS={"400":[[1101,100],[1102,100]]}。
// 配置只对原生全零表生效，不覆盖原生有权重的套装、位置或星级。
func nativeRuneSuitRows(dropID int, rows [][]json.RawMessage) ([][]json.RawMessage, error) {
	zero := true
	for _, row := range rows {
		var weight int64
		if len(row) != 2 || json.Unmarshal(row[1], &weight) != nil || weight < 0 {
			return nil, errors.New("原生契印套装权重无效")
		}
		if weight > 0 {
			zero = false
		}
	}
	if !zero {
		return rows, nil
	}
	var configured map[int][][]int64
	if raw := os.Getenv("HS_RUNE_FALLBACK_POOLS"); raw == "" || json.Unmarshal([]byte(raw), &configured) != nil {
		return nil, errors.New("原生契印套装权重全零，需要明确HS_RUNE_FALLBACK_POOLS运营候选池")
	}
	policy := configured[dropID]
	if len(policy) == 0 {
		return nil, errors.New("原生契印掉落编号尚无运营候选池")
	}
	result := [][]json.RawMessage{}
	seen := map[int64]bool{}
	for _, row := range policy {
		if len(row) != 2 || row[0] <= 0 || row[1] <= 0 || seen[row[0]] {
			return nil, errors.New("本服契印候选池编号、权重或重复项无效")
		}
		seen[row[0]] = true
		id, _ := json.Marshal(row[0])
		weight, _ := json.Marshal(row[1])
		result = append(result, []json.RawMessage{id, weight})
	}
	return result, nil
}
