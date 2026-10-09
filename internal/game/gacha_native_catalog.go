package game

import (
	_ "embed"
	"encoding/json"
)

//go:embed gacha_native_catalog.json
var gachaNativeCatalogRaw []byte

type nativeDrawPool struct {
	Cost        []int                 `json:"cost"`
	Limit       int                   `json:"limit"`
	Guarantee   int                   `json:"guarantee"`
	Activity    int                   `json:"activity"`
	Weights     map[int]map[int]int64 `json:"weights"`
	ResetGroups []int                 `json:"reset_groups"`
}

var androidNativeDrawPools = func() map[int]nativeDrawPool {
	var doc struct {
		Pools map[int]nativeDrawPool `json:"pools"`
	}
	if json.Unmarshal(gachaNativeCatalogRaw, &doc) != nil || len(doc.Pools) != 35 {
		panic("Android全卡池权重目录无效")
	}
	return doc.Pools
}()

func init() {
	for id, pool := range androidNativeDrawPools {
		cfg := androidGachaPools[id]
		cfg.PoolID, cfg.CostMaterial, cfg.CostCount = id, pool.Cost[0], pool.Cost[1]
		cfg.LimitCount, cfg.GuaranteeRarity = pool.Limit, pool.Guarantee
		if len(cfg.Cards) == 0 {
			seen := map[int]bool{}
			for _, weights := range pool.Weights {
				for cid := range weights {
					if !seen[cid] {
						cfg.Cards = append(cfg.Cards, gachaCardEntry{CardID: cid, Rarity: androidCardGrowth.Cards[cid].Rarity})
						seen[cid] = true
					}
				}
			}
		}
		androidGachaPools[id] = cfg
	}
}
