package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type collectionFusionRule struct {
	Comfort int     `json:"comfort"`
	Cost    []int64 `json:"cost"`
	Groups  []struct {
		Selector json.RawMessage `json:"selector"`
		Weight   int64           `json:"weight"`
		Wish     float64         `json:"wish_probability"`
	} `json:"groups"`
}

// Android只导出了furniture_compose_group的选择器，APK没有选择器解释器。
// 本服候选池必须由运营显式配置；键是“档位:从1开始的组号”，值是[材料编号,正权重]列表。
// 例如某个测试池{"3:2":[[6031001,1]]}，不是项目默认值，也不是原服分布证据。
func collectionFusionPools() (map[string][][2]int64, error) {
	raw := strings.TrimSpace(os.Getenv("HS_FURNITURE_FUSION_POOLS"))
	if raw == "" {
		return nil, runeReject("RET_HOUSE_COMPOSE_FURNITURE_POOL_ID_NOT_VALID", "缺少明确授权的本服家具融合候选池")
	}
	var pools map[string][][2]int64
	if json.Unmarshal([]byte(raw), &pools) != nil || len(pools) == 0 {
		return nil, errors.New("家具融合候选池配置格式无效")
	}
	return pools, nil
}

func collectionFusionDraw(p Progress, tier int, rule collectionFusionRule) (int, error) {
	pools, e := collectionFusionPools()
	if e != nil {
		return 0, e
	}
	groupWeights := []int64{}
	for i, group := range rule.Groups {
		groupWeights = append(groupWeights, group.Weight)
		if group.Weight == 0 {
			continue
		}
		rows := pools[fmt.Sprintf("%d:%d", tier, i+1)]
		if len(rows) == 0 {
			return 0, runeReject("RET_HOUSE_COMPOSE_FURNITURE_POOL_ID_NOT_VALID", "家具融合分组候选池为空")
		}
		seen := map[int64]bool{}
		for _, row := range rows {
			if row[0] <= 0 || row[0] > math.MaxInt32 || row[1] <= 0 || seen[row[0]] {
				return 0, errors.New("家具融合候选池材料或权重无效")
			}
			mat, known := androidShop.Materials[int(row[0])]
			if !known || (mat.Type != 6 && int(row[0]) != 6999999) {
				return 0, errors.New("家具融合候选池包含非家具奖励")
			}
			if mat.Type == 6 {
				furniture, exists := androidCollection.Furniture[mat.Target]
				if !exists || furniture.Type == 99 || furniture.Material != int(row[0]) {
					return 0, errors.New("家具融合候选池包含不可发放的家具模板")
				}
			}
			seen[row[0]] = true
		}
		if group.Wish > 0 && p.Collection.Wishlist != 0 && !seen[int64(p.Collection.Wishlist)] {
			return 0, errors.New("家具融合概率提升组不包含当前心愿单目标")
		}
	}
	i, e := shopWeightedIndex(groupWeights)
	if e != nil {
		return 0, e
	}
	group := rule.Groups[i]
	rows := pools[fmt.Sprintf("%d:%d", tier, i+1)]
	excludeWish := false
	if group.Wish > 0 && p.Collection.Wishlist != 0 {
		if len(rows) < 2 {
			return 0, errors.New("家具融合概率提升组缺少非心愿单候选")
		}
		up, e := shopProbabilityChoice(group.Wish)
		if e != nil {
			return 0, e
		}
		if up {
			return p.Collection.Wishlist, nil
		}
		excludeWish = true
	}
	weights := []int64{}
	for _, row := range rows {
		weight := row[1]
		if excludeWish && int(row[0]) == p.Collection.Wishlist {
			weight = 0
		}
		weights = append(weights, weight)
	}
	j, e := shopWeightedIndex(weights)
	if e != nil {
		return 0, e
	}
	return int(rows[j][0]), nil
}

func (s *Service) collectionFusionRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("家具融合需要回调和原料清单")
	}
	callback, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("家具融合缺少回调")
	}
	var inputs map[int]int64
	if json.Unmarshal(args[1], &inputs) != nil || len(inputs) == 0 {
		return nil, errors.New("家具融合原料清单无效")
	}
	box := map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		f, e := collectionWorkshop(p)
		if e != nil {
			return e
		}
		if !f.ComposeUnlocked {
			return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_UNLOCK", "家具融合尚未解锁")
		}
		ids := []int{}
		count := int64(0)
		comfort := int64(0)
		for mid, n := range inputs {
			mat, known := androidShop.Materials[mid]
			furniture, exists := androidCollection.Furniture[mat.Target]
			if !known || mat.Type != 6 || !exists || furniture.Type == 99 || n <= 0 || n > int64(androidCraft.FusionInputCount) || furniture.Comfort <= 0 {
				return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_VALID", "家具融合原料必须是有舒适度的真实家具")
			}
			count += n
			comfort += int64(furniture.Comfort) * n
			ids = append(ids, mid)
		}
		if count != int64(androidCraft.FusionInputCount) {
			return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_VALID", "家具融合必须恰好使用原生3件家具")
		}
		tier := 0
		for id, rule := range androidCraft.Fusion {
			if int64(rule.Comfort) <= comfort && (tier == 0 || rule.Comfort > androidCraft.Fusion[tier].Comfort) {
				tier = id
			}
		}
		if tier == 0 {
			return runeReject("RET_HOUSE_COMPOSE_FURNITURE_NOT_VALID", "家具融合舒适度不足原生最低门槛")
		}
		rule := androidCraft.Fusion[tier]
		// 在扣资产前校验全部候选分组；最终任一步失败仍由updateProgress整体回滚。
		result, e := collectionFusionDraw(*p, tier, rule)
		if e != nil {
			return e
		}
		if len(rule.Cost) != 2 || rule.Cost[0] <= 0 || rule.Cost[1] < 0 {
			return errors.New("家具融合原生成本无效")
		}
		if e := collectionSpend(p, int(rule.Cost[0]), rule.Cost[1]); e != nil {
			return e
		}
		sort.Ints(ids)
		for _, mid := range ids {
			if e := collectionConsumeCraft(p, mid, inputs[mid]); e != nil {
				return e
			}
		}
		changes := map[int]int64{}
		cards := []string{}
		if e := grantNativeItem(p, result, 1, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
			return e
		}
		box = map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cards)}
		syncCollectionHandbook(p)
		return nil
	})
	if e != nil {
		var reject *runeBusinessError
		if !errors.As(e, &reject) {
			return nil, e
		}
		code, known := androidCollection.Errors[reject.Name]
		if !known {
			return nil, errors.New("家具融合错误码缺失")
		}
		return []Push{Callback(callback, []any{code, box})}, nil
	}
	out := []Push{materialManagerPush(c), knowledgePush(c)}
	out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)...)
	return append(out, Callback(callback, []any{RetSuccess, box})), nil
}
