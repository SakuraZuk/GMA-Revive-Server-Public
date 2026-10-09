package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

//go:embed shop_catalog.json
var shopCatalogRaw []byte

type commodityRule struct {
	ID                int                   `json:"commodity_id"`
	Type              int                   `json:"commodity_type"`
	Item              int                   `json:"item_id"`
	Quantity          int64                 `json:"item_num"`
	Shop              int                   `json:"shop_id"`
	Coin              int                   `json:"coin_id"`
	Prices            []int64               `json:"coin_price"`
	Discount          float64               `json:"discount_ratio"`
	DiscountLimit     *int64                `json:"discount_limit"`
	DiscountRange     []float64             `json:"discount_ratio_range"`
	Daily             int64                 `json:"limit_perday"`
	Weekly            int64                 `json:"limit_perweek"`
	Monthly           int64                 `json:"limit_permonth"`
	Total             int64                 `json:"total_buy_limit"`
	Activity          int64                 `json:"activity_buy_limit"`
	Begin             []int64               `json:"begin_time"`
	End               []int64               `json:"end_time"`
	Unlock            [][][]json.RawMessage `json:"unlock_condition"`
	Invalid           [][][]json.RawMessage `json:"invalid_condition"`
	InvalidHours      float64               `json:"invalid_time"`
	OpenDays          *int                  `json:"open_server_days"`
	Batch             int                   `json:"can_buy_by_batch"`
	BatchLimit        int64                 `json:"batch_buy_limit"`
	Bonus             int                   `json:"give_bonus_id"`
	RuneList          []int                 `json:"rune_commodity_list"`
	SubscribeDays     int64                 `json:"subscribe_days"`
	RecommendInterval int64                 `json:"recommend_interval"`
}
type shopRule struct {
	Begin  []int64 `json:"begin_time"`
	End    []int64 `json:"end_time"`
	System string  `json:"lock_system_name"`
}
type shopCatalog struct {
	Commodities map[int]commodityRule `json:"commodities"`
	Shops       map[int]shopRule      `json:"shops"`
	Materials   map[int]struct {
		Type   int   `json:"type"`
		Sub    int   `json:"stype"`
		Target int   `json:"target_id"`
		Limit  int64 `json:"limit_count"`
	} `json:"materials"`
	Bonuses map[int]struct {
		Fixed         [][]int64         `json:"fixed_items"`
		Inner         json.RawMessage   `json:"inner_bonus_id"`
		RandomItems   []json.RawMessage `json:"random_items"`
		RandomRunes   []json.RawMessage `json:"random_runes"`
		RandomLibs    []json.RawMessage `json:"random_item_libs"`
		Begin         []int64           `json:"begin_time"`
		End           []int64           `json:"end_time"`
		Lucky         int               `json:"lucky_rule_id"`
		ActivityBonus int               `json:"activity_bonus_id"`
	} `json:"bonuses"`
	GiftBoxes     map[int][]ShopGiftBoxRule       `json:"gift_boxes"`
	RuneDrops     map[int]shopRuneDrop            `json:"rune_drops"`
	ItemLibraries map[int]shopItemLibrary         `json:"item_libs"`
	LibraryData   map[int]map[int]json.RawMessage `json:"item_libs_data"`
	MailTemplates map[int]struct {
		Title   string `json:"mail_title"`
		Content string `json:"mail_content"`
	} `json:"mail_templates"`
	Errors map[string]int `json:"errors"`
}

var androidShop = func() shopCatalog {
	var c shopCatalog
	if err := json.Unmarshal(shopCatalogRaw, &c); err != nil {
		panic(err)
	}
	if len(c.Commodities) != 881 || len(c.Shops) != 35 {
		panic("Android商店目录无效")
	}
	return c
}()

type CommodityDetail struct {
	ID             int   `json:"commodity_id"`
	Time           int64 `json:"time"`
	Daily          int64 `json:"daily_buy_times"`
	Weekly         int64 `json:"weekly_buy_times"`
	Monthly        int64 `json:"monthly_buy_times"`
	Total          int64 `json:"total_buy_times"`
	Discount       int64 `json:"discount_buy_times"`
	Activity       int64 `json:"activity_buy_times"`
	SubscribeCount int64 `json:"subscribe_count"`
	SubscribeTime  int64 `json:"subscribe_time"`
	RecommendTime  int64 `json:"recommend_time"`
	// 内部日界不作为Avatar未知字段下发。
	LastPurchase   int64               `json:"server_last_purchase,omitempty"`
	SubscribeJobs  []ShopSubscription  `json:"server_subscription_jobs,omitempty"`
	Recommendation *ShopRecommendation `json:"server_recommendation,omitempty"`
}

func refreshCommodity(d *CommodityDetail, now time.Time) error {
	if d.LastPurchase > now.Unix() {
		return errors.New("商店存档时间回拨")
	}
	if d.ID <= 0 || d.Daily < 0 || d.Weekly < 0 || d.Monthly < 0 || d.Total < 0 || d.Discount < 0 || d.Activity < 0 {
		return errors.New("商品次数存档无效")
	}
	if d.LastPurchase == 0 {
		return nil
	}
	z := time.FixedZone("北京时间", 8*3600)
	a, b := time.Unix(d.LastPurchase, 0).In(z), now.In(z)
	reset := false
	if a.Format("2006-01-02") != b.Format("2006-01-02") {
		d.Daily = 0
		reset = true
	}
	ay, aw := a.ISOWeek()
	by, bw := b.ISOWeek()
	if ay != by || aw != bw {
		d.Weekly = 0
		reset = true
	}
	if a.Format("2006-01") != b.Format("2006-01") {
		d.Monthly = 0
		reset = true
	}
	if reset {
		d.Discount = 0
	}
	return nil
}

func commodityProperties(p Progress, now time.Time) map[string]any {
	out := map[string]any{}
	for id, d := range p.CommodityDetails {
		_ = refreshCommodity(&d, now)
		b, _ := json.Marshal(d)
		var row map[string]any
		_ = json.Unmarshal(b, &row)
		delete(row, "server_last_purchase")
		delete(row, "server_subscription_jobs")
		delete(row, "server_recommendation")
		out[strconv.Itoa(id)] = row
	}
	return out
}

// dungeon_check原生组合为组间且、组内或。未知条件拒绝，不当作放行。
func shopConditions(conditions [][][]json.RawMessage, p Progress, level int) (bool, error) {
	for _, group := range conditions {
		if len(group) == 0 {
			continue
		}
		matched := false
		unknown := false
		for _, c := range group {
			if len(c) != 2 {
				return false, errors.New("商店条件结构无效")
			}
			var kind, value int
			if json.Unmarshal(c[0], &kind) != nil {
				unknown = true
				continue
			}
			if kind == 12 {
				var value string
				if json.Unmarshal(c[1], &value) != nil {
					return false, errors.New("通关次数条件格式无效")
				}
				parts := strings.Split(value, "#")
				if len(parts) != 3 {
					return false, errors.New("通关次数条件格式无效")
				}
				typeID, e1 := strconv.Atoi(parts[0])
				subtype, e2 := strconv.Atoi(parts[1])
				need, e3 := strconv.Atoi(parts[2])
				if e1 != nil || e2 != nil || e3 != nil || typeID <= 0 || subtype < 0 || need < 0 {
					return false, errors.New("通关次数条件数值无效")
				}
				matched = matched || activityConditionCount(p, typeID, subtype) >= need
				continue
			}
			if kind == 11 {
				var text string
				if json.Unmarshal(c[1], &text) != nil {
					return false, errors.New("累计材料条件无效")
				}
				parts := strings.Split(text, "#")
				if len(parts) != 2 {
					return false, errors.New("累计材料条件格式无效")
				}
				id, e1 := strconv.Atoi(parts[0])
				need, e2 := strconv.ParseInt(parts[1], 10, 64)
				if e1 != nil || e2 != nil || id <= 0 || need < 0 {
					return false, errors.New("累计材料条件数值无效")
				}
				matched = matched || p.Materials[id].Total >= need
				continue
			}
			if json.Unmarshal(c[1], &value) != nil {
				unknown = true
				continue
			}
			switch kind {
			case 1:
				matched = matched || containsInt(p.ClearedDungeons, value)
			case 2:
				matched = matched || level >= value
			default:
				unknown = true
			}
		}
		if !matched {
			if unknown {
				return false, errors.New("商店条件类型尚未接线")
			}
			return false, nil
		}
	}
	return true, nil
}

func shopTimeWindow(begin, end []int64, now int64) string {
	sum := func(v []int64) int64 {
		var n int64
		for _, x := range v {
			n += x
		}
		return n
	}
	if len(begin) > 0 && now < sum(begin) {
		return "RET_SHOP_ACTIVITY_NOT_BEGIN"
	}
	if len(end) > 0 && now > sum(end) {
		return "RET_SHOP_ACTIVITY_HAS_ENDED"
	}
	return ""
}

func buyCommodity(p *Progress, id int, count int64, selectID int, level int, now time.Time) (map[string]any, error) {
	if p.AvatarLevel > 0 {
		level = p.AvatarLevel
	}
	r, ok := androidShop.Commodities[id]
	if !ok {
		return nil, runeReject("RET_SHOP_INVALID", "商品不存在")
	}
	shop, ok := androidShop.Shops[r.Shop]
	if !ok {
		return nil, errors.New("商品所属商店不存在")
	}
	if shop.System != "" {
		if _, ok := p.UnlockSystems[shop.System]; !ok {
			return nil, runeReject("RET_SHOP_NO_OPEN", "所属系统尚未解锁")
		}
	}
	permanent, err := activityShopAvailable(*p, r.Shop, level, now)
	if err != nil {
		return nil, err
	}
	if !permanent {
		for _, w := range [][2][]int64{{r.Begin, r.End}, {shop.Begin, shop.End}} {
			if e := shopTimeWindow(w[0], w[1], now.Unix()); e != "" {
				return nil, runeReject(e, "商品不在开放期")
			}
		}
	}
	if r.OpenDays != nil && !shopPermanentOpenDays(r) {
		if err := checkShopServerOpenDays(*r.OpenDays, now); err != nil {
			return nil, err
		}
	}
	if (r.Type != 1 && r.Type != 2 && r.Type != 3 && r.Type != 4) || len(r.RuneList) > 0 || (r.Type != 3 && len(r.DiscountRange) > 0) {
		return nil, runeReject("RET_SHOP_INVALID", "商品专属业务门槛尚未接线")
	}
	selfSelect := androidShop.Materials[r.Item].Type == 7 && androidShop.Materials[r.Item].Sub == 3
	if (!selfSelect && selectID != 0) || (selfSelect && selectID <= 0) {
		return nil, runeReject("RET_SHOP_INVALID", "商品自选材料参数不符合类型")
	}
	allowed, err := shopConditions(r.Unlock, *p, level)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, runeReject("RET_FORBID_SHOP", "商品尚未解锁")
	}
	if len(r.Invalid) > 0 {
		invalid, err := shopConditions(r.Invalid, *p, level)
		if err != nil {
			return nil, err
		}
		if invalid {
			return nil, runeReject("RET_SHOP_INVALID", "商品失效条件已满足")
		}
	}
	if count <= 0 || count > 9999 {
		return nil, runeReject("RET_SHOP_BATCH_BUY_LIMIT", "商品数量无效")
	}
	if count > 1 && (r.Batch == 0 || r.BatchLimit <= 0 || count > r.BatchLimit) {
		return nil, runeReject("RET_SHOP_BATCH_BUY_LIMIT", "商品批量超限")
	}
	d := p.CommodityDetails[id]
	if d.ID == 0 && r.InvalidHours > 0 {
		return nil, runeReject("RET_SHOP_INVALID", "限时商品尚无服务端上架记录")
	}
	if d.ID == 0 {
		d = CommodityDetail{ID: id, Time: now.Unix()}
	}
	if err := refreshCommodity(&d, now); err != nil {
		return nil, err
	}
	if r.InvalidHours > 0 && float64(now.Unix()-d.Time) >= r.InvalidHours*3600 {
		return nil, runeReject("RET_SHOP_INVALID", "商品有效期已结束")
	}
	for _, lim := range [][2]int64{{d.Daily, r.Daily}, {d.Weekly, r.Weekly}, {d.Monthly, r.Monthly}, {d.Total, r.Total}, {d.Activity, r.Activity}} {
		if lim[0] > math.MaxInt64-count {
			return nil, errors.New("购买次数溢出")
		}
		if lim[1] > 0 && count > lim[1]-lim[0] {
			return nil, runeReject("RET_SHOP_GOOD_HAS_SOLD_OUT", "商品限购次数不足")
		}
	}
	if len(r.Prices) == 0 {
		return nil, errors.New("商品价格配置为空")
	}
	index := d.Daily
	if index >= int64(len(r.Prices)) {
		index = int64(len(r.Prices) - 1)
	}
	unit := r.Prices[index]
	if unit < 0 {
		unit = 0
	}
	discount := r.Type != 3 && r.Discount < 1 && (r.DiscountLimit == nil || d.Discount < *r.DiscountLimit)
	if r.Type == 3 {
		if e := validShopRecommendation(d, now); e != nil {
			return nil, e
		}
		unit = d.Recommendation.Price
	}
	if discount {
		if r.Discount <= 0 || math.IsNaN(r.Discount) || math.IsInf(r.Discount, 0) {
			return nil, errors.New("商品折扣无效")
		}
		unit = int64(float64(unit) * r.Discount)
	}
	if unit > math.MaxInt64/count {
		return nil, errors.New("购买价格溢出")
	}
	cost := unit * count
	coin := p.Materials[r.Coin]
	if coin.Count < cost {
		return nil, runeReject("RES_SHOP_NOT_ENOUGH_COIN", "商店货币不足")
	}
	if r.Quantity <= 0 || r.Quantity > math.MaxInt64/count {
		return nil, errors.New("商品奖励数量溢出")
	}
	amount := r.Quantity * count
	changes := map[int]int64{}
	cardIDs := []string{}
	runesBefore := map[string]bool{}
	for uuid := range p.Runes {
		runesBefore[uuid] = true
	}
	// 扣款、礼盒展开、全部资产与次数同属玩家事务。
	coin.Count -= cost
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	if cost > 0 {
		p.Materials[r.Coin] = coin
	}
	if r.Type == 4 {
		for i := int64(0); i < amount; i++ {
			if err := grantShopGiftBox(p, r.Item, now, changes, &cardIDs); err != nil {
				return nil, err
			}
		}
	} else if selfSelect {
		if err := grantNativeSelectedBonus(p, androidShop.Materials[r.Item].Target, amount, selectID, level, now, changes, &cardIDs, 0); err != nil {
			return nil, err
		}
	} else {
		if err := grantNativeItem(p, r.Item, amount, level, now, changes, &cardIDs, 0); err != nil {
			return nil, err
		}
	}
	if r.Bonus != 0 {
		if err := grantNativeBonusAssets(p, r.Bonus, count, level, now, changes, &cardIDs, 0); err != nil {
			return nil, err
		}
	}
	if r.Type == 2 {
		if r.SubscribeDays != 30 || count != 1 || amount != 1 {
			return nil, errors.New("订阅商品天数或购买份数无效")
		}
		if d.SubscribeCount == math.MaxInt64 {
			return nil, errors.New("订阅次数溢出")
		}
		d.SubscribeCount++
		d.SubscribeTime = nextShopDay(now)
		d.SubscribeJobs = append(d.SubscribeJobs, ShopSubscription{Sequence: d.Total + 1, StartDay: nextShopDay(now), Delivered: 1, Days: r.SubscribeDays, Bonus: androidShop.Materials[r.Item].Target})
	}
	box := map[string]any{"__custom_type": "box.box", "materials": changes, "cards": cardListWire(p, cardIDs)}
	awardedRunes := map[string]Rune{}
	for uuid, rune := range p.Runes {
		if !runesBefore[uuid] {
			awardedRunes[uuid] = rune
		}
	}
	if len(awardedRunes) > 0 {
		box["runes"] = runeMgrProperties(awardedRunes)
	}
	d.Daily += count
	d.Weekly += count
	d.Monthly += count
	d.Total += count
	d.Activity += count
	if discount {
		if d.Discount > math.MaxInt64-count {
			return nil, errors.New("折扣计数溢出")
		}
		d.Discount += count
	}
	d.LastPurchase = now.Unix()
	if r.Type == 3 {
		d.Recommendation.State = 3
	}
	if p.CommodityDetails == nil {
		p.CommodityDetails = map[int]CommodityDetail{}
	}
	p.CommodityDetails[id] = d
	return box, nil
}

func (s *Service) buyCommodityRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("购买商品需要编号、数量和自选材料编号")
	}
	var id, selected int
	var count int64
	if json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &count) != nil || json.Unmarshal(args[2], &selected) != nil || id <= 0 {
		return nil, errors.New("购买商品参数无效")
	}
	var box map[string]any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		var e error
		box, e = buyCommodity(p, id, count, selected, c.SelectedAvatarUnsafe().Info.Level, s.Now())
		return e
	})
	if err != nil {
		var rejected *runeBusinessError
		if !errors.As(err, &rejected) {
			return nil, err
		}
		code, ok := androidShop.Errors[rejected.Name]
		if !ok {
			return nil, errors.New("商店错误码缺失")
		}
		return []Push{push("Avatar", "on_buy_commodity", code, map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}, id, selected)}, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	out := []Push{materialManagerPush(c), cardMgrPush(c), knowledgePush(c), runePush(c), push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(p)}), push("Avatar", "client_prop_changed", []any{"power", p.Power}), push("Avatar", "client_prop_changed", []any{"gift_box_info", p.GiftBoxInfo}), push("Avatar", "client_prop_changed", []any{"owned_head_box", p.OwnedHeadBox}), push("Avatar", "client_prop_changed", []any{"commodity_detail_info", commodityProperties(p, s.Now())})}
	if p.Collection != nil {
		out = append(out, collectionPushes(p, s.Now(), false)...)
	}
	if materials, ok := box["materials"].(map[int]int64); ok && materials[206001] > 0 && p.Activities.Wangyan != nil {
		out = append(out, push("Avatar", "client_prop_changed", []any{"wangyan_game", activityWire(wangyanProperties(p.Activities.Wangyan, s.Now()))}))
	}
	out = append(out, push("Avatar", "client_prop_changed", []any{"recommend_gifts", recommendationProperties(p)}))
	return append(out, push("Avatar", "on_buy_commodity", RetSuccess, box, id, selected)), nil
}

// 即时礼盒只按Android固定奖励展开；未知随机/活动条件不得替换成固定奖。
func grantShopItem(p *Progress, id int, amount int64, now time.Time, changes map[int]int64, cardIDs *[]string, depth int) error {
	return grantNativeItem(p, id, amount, 0, now, changes, cardIDs, depth)
}
func grantNativeItem(p *Progress, id int, amount int64, level int, now time.Time, changes map[int]int64, cardIDs *[]string, depth int) error {
	if depth > 8 || amount <= 0 {
		return errors.New("商店奖励嵌套或数量无效")
	}
	mat, ok := androidShop.Materials[id]
	if !ok {
		return errors.New("商店奖励材料不存在")
	}
	switch mat.Type {
	case 1:
		if id != 1 || amount > math.MaxInt32 || p.Power.Value < 0 || int64(p.Power.Value) > math.MaxInt32-amount {
			return errors.New("商店体力奖励数量无效或溢出")
		}
		p.settlePowerRecovery(now)
		if int64(p.Power.Value) > math.MaxInt32-amount {
			return errors.New("商店体力奖励溢出")
		}
		p.Power.Value += int(amount)
		if p.Power.Max > 0 && p.Power.Value >= p.Power.Max {
			p.Power.LastTime = float64(now.UnixNano()) / 1e9
		}
		changes[id] += amount
	case 2:
		if changes[id] > math.MaxInt64-amount {
			return errors.New("经验奖励数量溢出")
		}
		switch mat.Sub {
		case 1:
			if id != 2 {
				return errors.New("馆主经验材料编号无效")
			}
			if err := grantAvatarExp(p, amount, now); err != nil {
				return err
			}
		case 2:
			if id != 3 {
				return errors.New("幻书经验材料编号无效")
			}
			// 原生box.get_card_exps由card_exp_receiver逐UUID分配满额；调用者在同事务结算。
		case 3:
			if id != 4 {
				return errors.New("知识储备材料编号无效")
			}
			if p.Materials == nil {
				p.Materials = map[int]Material{}
			}
			m := p.Materials[id]
			if m.Count < 0 || m.Total < 0 || m.Count > math.MaxInt64-amount || m.Total > math.MaxInt64-amount {
				return errors.New("知识储备奖励溢出")
			}
			m.ID = id
			m.Count += amount
			m.Total += amount
			p.Materials[id] = m
		default:
			return errors.New("经验奖励子类型不存在")
		}
		changes[id] += amount
	case 3, 4, 6, 11:
		// 本版type11为累计积分（新手群星52等），由material_mgr保存count/total。
		// 客户端check_material与score_bonus均读取该容器，不应当作未知材料拒绝。
		if mat.Type == 4 && mat.Sub == 20 {
			if id != 206001 || changes[id] > math.MaxInt64-amount {
				return errors.New("妄言补给材料编号或奖励数量无效")
			}
			// 原生 time_auto_attr.add 默认 auto=False：显式补给入真实
			// supply，允许超过自然回复上限，不写普通材料库存。
			actual, err := grantWangyanSupply(p, amount, now)
			if err != nil {
				return err
			}
			changes[id] += actual
			return nil
		}
		if mat.Type == 6 {
			f, known := androidCollection.Furniture[mat.Target]
			if !known || f.Type == 99 {
				return errors.New("商店家具材料模板无效")
			}
			if err := ensureCollection(p, now); err != nil {
				return err
			}
		}
		if p.Materials == nil {
			p.Materials = map[int]Material{}
		}
		m := p.Materials[id]
		if m.Count < 0 || m.Total < 0 || m.Count > math.MaxInt64-amount || m.Total > math.MaxInt64-amount || changes[id] > math.MaxInt64-amount {
			return errors.New("商品奖励余额溢出")
		}
		if mat.Limit > 0 && amount > mat.Limit-m.Count {
			return runeReject("RET_SHOP_BUY_MAX_COUNT_LIMIT", "商品材料达到容量上限")
		}
		m.ID = id
		m.Count += amount
		m.Total += amount
		p.Materials[id] = m
		changes[id] += amount
		if mat.Type == 6 {
			syncCollectionHandbook(p)
		}
	case 7:
		if mat.Sub != 1 {
			return runeReject("RET_SHOP_INVALID", "自选或存储礼盒业务尚未接线")
		}
		return grantNativeBonusAssets(p, mat.Target, amount, level, now, changes, cardIDs, depth+1)
	case 8:
		if _, ok := androidOath.Cards[mat.Target]; !ok {
			return errors.New("商品幻书模板不存在")
		}
		if amount > int64(androidCompose.MaxCards-len(p.Cards)) {
			return runeReject("RET_CARD_COUNT_REACH_MAX", "幻书背包容量不足")
		}
		for i := int64(0); i < amount; i++ {
			card := newCard(mat.Target, 1, now)
			appendOwnedCard(p, card)
			*cardIDs = append(*cardIDs, card.UUID)
		}
	case 9:
		if changes[id] > math.MaxInt64-amount {
			return errors.New("头像框奖励数量溢出")
		}
		if e := grantProfileHeadBox(p, id, mat.Target, amount, now); e != nil {
			return e
		}
		changes[id] += amount
	case 13:
		cardID := 0
		for candidate, appearance := range androidCardAppearances {
			if containsInt(appearance.Dresses, mat.Target) {
				if cardID != 0 {
					return errors.New("装束跨幻书模板歧义")
				}
				cardID = candidate
			}
		}
		if cardID == 0 {
			return errors.New("装束材料没有原生幻书归属")
		}
		if p.OwnedDresses == nil {
			p.OwnedDresses = map[int][]int{}
		}
		if !containsInt(p.OwnedDresses[cardID], mat.Target) {
			p.OwnedDresses[cardID] = append(p.OwnedDresses[cardID], mat.Target)
		}
		if changes[id] > math.MaxInt64-amount {
			return errors.New("装束奖励数量溢出")
		}
		changes[id] += amount
	default:
		return runeReject("RET_SHOP_INVALID", "商品材料业务尚未接线")
	}
	return nil
}
