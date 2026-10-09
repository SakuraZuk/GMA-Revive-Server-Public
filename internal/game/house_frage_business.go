package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"
)

// Android FF2A17AE/1EBB86C4及Avatar127E9B33的残页梦境状态；隐藏账本不发到客户端。
type HouseFrageSite struct {
	ID       int   `json:"site_id"`
	Card     int   `json:"card_id"`
	Fixed    int   `json:"fix_card_id"`
	Prop     int   `json:"random_prop"`
	Material int   `json:"prop_material_id"`
	Game     []int `json:"game_material_ids"`
	Opened   []int `json:"server_opened,omitempty" wire:"-"`
}
type HouseFrageStone struct {
	ID        int       `json:"material_id"`
	Time      int64     `json:"time"`
	Materials [][]int64 `json:"material_list"`
}
type HouseFrageState struct {
	ID                    int                     `json:"id"`
	Pos                   int                     `json:"pos"`
	Double                int                     `json:"double"`
	Up                    int                     `json:"up"`
	Slow                  int                     `json:"slow"`
	Total                 int                     `json:"total"`
	SSR                   int                     `json:"total_ssr"`
	Marks                 map[int]int             `json:"marks"`
	CurrentMarks          map[int]int             `json:"current_marks"`
	Sites                 map[int]*HouseFrageSite `json:"sites"`
	Stone                 HouseFrageStone         `json:"stone"`
	RewardDay             string                  `json:"reward_day"`
	PolicyVersion         string                  `json:"server_policy_version,omitempty"`
	ConsecutiveWithoutSSR int                     `json:"server_consecutive_without_ssr,omitempty"`
}

// 该连续计数是显式本服策略；历史Total/SSR没有事件顺序，不能推算连续遗漏。
func houseFragePolicyPerCount(w *HouseFrageState) (int, error) {
	if !remainingPolicyEnabled() {
		return w.Total, nil
	}
	if w.PolicyVersion != "local-20261008-v1" {
		w.PolicyVersion = "local-20261008-v1"
		w.ConsecutiveWithoutSSR = 0
	}
	if w.ConsecutiveWithoutSSR < 0 {
		return 0, errors.New("残页连续次数存档无效")
	}
	return w.ConsecutiveWithoutSSR, nil
}
func houseFragePolicyReward(w *HouseFrageState, rarity int) error {
	if !remainingPolicyEnabled() {
		return nil
	}
	if _, err := houseFragePolicyPerCount(w); err != nil {
		return err
	}
	if rarity == 4 {
		w.ConsecutiveWithoutSSR = 0
		return nil
	}
	if rarity < 1 || rarity > 3 {
		return errors.New("残页连续计数稀有度无效")
	}
	if w.ConsecutiveWithoutSSR == int(^uint(0)>>1) {
		return errors.New("残页连续次数溢出")
	}
	w.ConsecutiveWithoutSSR++
	return nil
}

func ensureHouseFrage(p *Progress) *HouseFrageState {
	if p.Activities.House == nil {
		p.Activities.House = &HouseFrageState{ID: 1, Marks: map[int]int{}, CurrentMarks: map[int]int{}, Sites: map[int]*HouseFrageSite{}, Stone: HouseFrageStone{Materials: [][]int64{}}}
	}
	if p.Materials == nil {
		p.Materials = map[int]Material{}
	}
	return p.Activities.House
}
func houseFrageProperties(p Progress) map[string]any {
	w := p.Activities.House
	if w == nil {
		return nil
	}
	return map[string]any{"house_frage_id": w.ID, "house_frage_pos": w.Pos, "house_frage_dice_count": p.Materials[401].Count, "house_frage_ctrl_dice_count": p.Materials[402].Count, "house_frage_double_count": w.Double, "house_frage_up_count": w.Up, "house_frage_mark_card": w.Marks, "house_frage_current_mark_card": w.CurrentMarks, "house_frage_slow_state": w.Slow, "house_frage_sites": w.Sites, "current_frage_stone": w.Stone}
}
func houseFrageLevel(p *Progress) (int, error) {
	if p.Collection == nil || p.Collection.Facilities[6].Level <= 0 {
		return 0, errors.New("残页梦境骰子桌尚未解锁")
	}
	level := p.Collection.Facilities[6].Level
	if len(activityData("facility_board_game", level)) == 0 {
		return 0, errors.New("骰子桌等级不存在")
	}
	return level, nil
}
func houseFrageFragment(card int) int {
	var id int
	_ = json.Unmarshal(androidActivities["card_to_fragment"][strconv.Itoa(card)], &id)
	return id
}
func houseFrageCards(rarity, level int) []int {
	ids := []int{}
	for key := range androidActivities["cards"] {
		id, _ := strconv.Atoi(key)
		r := activityData("cards", id)
		available := r.integer("card_house_available")
		if available > 0 && available <= level && !r.flag("disable") && r.integer("rarity") == rarity {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}
func houseFrageChoice(ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("残页梦境可选卡牌为空")
	}
	n, e := activityAmount([]int64{0, int64(len(ids) - 1)})
	if e != nil {
		return 0, e
	}
	return ids[n], nil
}
func houseFrageWeight(rows [][]int) ([]int, error) {
	weights := []int{}
	for _, row := range rows {
		if len(row) < 2 || row[len(row)-1] < 0 {
			return nil, errors.New("残页梦境权重配置无效")
		}
		weights = append(weights, row[len(row)-1])
	}
	i, e := activityWeighted(weights)
	if e != nil {
		return nil, e
	}
	return rows[i], nil
}
func houseFrageFloatWeight(weights []float64) (int, error) {
	scaled := []int{}
	for _, v := range weights {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > math.MaxInt32/100000 {
			return 0, errors.New("残页梦境浮点权重无效")
		}
		scaled = append(scaled, int(math.Round(v*100000)))
	}
	return activityWeighted(scaled)
}
func houseFrageSSRWeight(r activityRow, perCount int) float64 {
	base := float64(r.integer("ssr_weight"))
	if perCount <= 0 {
		return base
	}
	keys := []int{}
	for key := range androidActivities["card_house_waypoint_weight"] {
		k, _ := strconv.Atoi(key)
		keys = append(keys, k)
	}
	sort.Ints(keys)
	if len(keys) == 0 {
		return base
	}
	match := keys[len(keys)-1]
	for _, k := range keys {
		if perCount < k {
			match = k
			break
		}
	}
	var factor float64
	_ = json.Unmarshal(activityData("card_house_waypoint_weight", match)["weight_factor"], &factor)
	if factor > 0 {
		return base * factor
	}
	return base
}
func houseFrageRollSite(p *Progress, id, level, living int) (*HouseFrageSite, error) {
	w := ensureHouseFrage(p)
	perCount, err := houseFragePolicyPerCount(w)
	if err != nil {
		return nil, err
	}
	r := activityData("card_house", id)
	out := &HouseFrageSite{ID: id, Game: []int{}}
	args := activityData("card_house_params", w.ID)
	switch r.integer("site_type") {
	case 1:
		card, e := houseFrageChoice(houseFrageCards(1, level))
		if e != nil {
			return nil, e
		}
		out.Card = card
	case 2:
		kinds := []int{0, -1, 2, 3, 4}
		weights := []float64{float64(r.integer("random_weight")), float64(r.integer("fixed_card_weight")), float64(r.integer("r_weight")), float64(r.integer("sr_weight")), houseFrageSSRWeight(r, perCount)}
		if living > 0 {
			var probs [][]float64
			_ = json.Unmarshal(activityData("house_frage_params", 1)["card_count_probs"], &probs)
			index := living - 1
			if index >= len(probs) {
				index = len(probs) - 1
			}
			if index < 0 || len(probs[index]) != 2 {
				return nil, errors.New("残页梦境入住概率配置无效")
			}
			total := float64(0)
			for _, v := range weights {
				total += v
			}
			sr, ssr := float64(int(probs[index][0]*total)), float64(int(probs[index][1]*total))
			weights[2] -= sr + ssr
			weights[3] += sr
			weights[4] += ssr
		}
		i, e := houseFrageFloatWeight(weights)
		if e != nil {
			return nil, e
		}
		kind := kinds[i]
		if kind == 0 {
			props := [][]int{{1, args.integer("double_prop_weight")}, {2, args.integer("up_prop_weight")}, {3, args.integer("move_prop_weight")}}
			row, e := houseFrageWeight(props)
			if e != nil {
				return nil, e
			}
			out.Prop = row[0]
		} else if kind == -1 {
			out.Fixed = r.integer("fixed_card_id")
			out.Card = out.Fixed
		} else {
			pool := houseFrageCards(kind, level)
			mark := w.Marks[kind]
			if mark > 0 && containsInt(pool, mark) {
				var chance float64
				field := "mark_sr_prob"
				if kind == 4 {
					field = "mark_ssr_prob"
				}
				_ = json.Unmarshal(args[field], &chance)
				yes, e := activityChance(chance)
				if e != nil {
					return nil, e
				}
				if yes || len(pool) == 1 {
					out.Card = mark
				} else {
					others := []int{}
					for _, card := range pool {
						if card != mark {
							others = append(others, card)
						}
					}
					out.Card, e = houseFrageChoice(others)
					if e != nil {
						return nil, e
					}
				}
			} else {
				out.Card, e = houseFrageChoice(pool)
				if e != nil {
					return nil, e
				}
			}
		}
	case 6:
		var rows [][]int
		_ = json.Unmarshal(r["prop_infos"], &rows)
		row, e := houseFrageWeight(rows)
		if e != nil {
			return nil, e
		}
		out.Material = row[0]
	case 7:
		bag := map[int]float64{}
		var props [][]int
		_ = json.Unmarshal(r["prop_infos"], &props)
		for _, row := range props {
			if len(row) != 2 {
				return nil, errors.New("残页梦境游戏材料权重无效")
			}
			bag[row[0]] = float64(row[1])
		}
		for _, rarity := range []int{2, 3, 4} {
			weight := float64(r.integer("r_weight"))
			if rarity == 3 {
				weight = float64(r.integer("sr_weight"))
			}
			if rarity == 4 {
				weight = houseFrageSSRWeight(r, perCount)
			}
			if weight <= 0 {
				continue
			}
			card, e := houseFrageChoice(houseFrageCards(rarity, level))
			if e != nil {
				return nil, e
			}
			mid := houseFrageFragment(card)
			if mid <= 0 {
				return nil, errors.New("残页梦境卡牌残页映射不存在")
			}
			bag[mid] = weight
		}
		if fixed := r.integer("fixed_card_id"); fixed > 0 {
			mid := houseFrageFragment(fixed)
			if mid > 0 {
				bag[mid] = float64(r.integer("fixed_card_weight"))
			}
		}
		for len(bag) > 0 && len(out.Game) < 3 {
			ids := []int{}
			for mid := range bag {
				ids = append(ids, mid)
			}
			sort.Ints(ids)
			weights := []float64{}
			for _, mid := range ids {
				weights = append(weights, bag[mid])
			}
			i, e := houseFrageFloatWeight(weights)
			if e != nil {
				return nil, e
			}
			out.Game = append(out.Game, ids[i])
			delete(bag, ids[i])
		}
	}
	return out, nil
}
func houseFrageRoll(p *Progress, level int) error {
	w := ensureHouseFrage(p)
	living := 0
	for _, room := range p.Collection.Rooms {
		for _, card := range room.Cards {
			if card > 0 {
				living++
			}
		}
	}
	sites := map[int]*HouseFrageSite{}
	keys := []int{}
	for key := range androidActivities["card_house"] {
		id, _ := strconv.Atoi(key)
		keys = append(keys, id)
	}
	sort.Ints(keys)
	for _, id := range keys {
		site, e := houseFrageRollSite(p, id, level, living)
		if e != nil {
			return e
		}
		sites[id] = site
	}
	w.Sites = sites
	w.CurrentMarks = map[int]int{}
	for k, v := range w.Marks {
		w.CurrentMarks[k] = v
	}
	w.Pos = 1
	w.Double = 0
	w.Up = 0
	w.Slow = 0
	return nil
}
func houseFrageAdditions(p *Progress, level, rarity int) (int, error) {
	var effects [][]json.RawMessage
	if json.Unmarshal(activityData("facility_board_game", level)["upgrade_effect"], &effects) != nil {
		return 0, errors.New("骰子桌设施奖励配置无效")
	}
	want := strconv.Itoa(13 + rarity)
	add := 0
	for _, pair := range effects {
		if len(pair) != 2 {
			return 0, errors.New("骰子桌设施效果结构无效")
		}
		var kind string
		_ = json.Unmarshal(pair[0], &kind)
		if kind == want {
			var n int
			if json.Unmarshal(pair[1], &n) != nil || n < 0 {
				return 0, errors.New("骰子桌设施残页加成无效")
			}
			add += n
		}
	}
	return add, nil
}
func houseFrageCardBonus(p *Progress, site *HouseFrageSite, level int) (int, int64, bool, error) {
	w := ensureHouseFrage(p)
	args := activityData("card_house_params", w.ID)
	rarity := activityData("cards", site.Card).integer("rarity")
	fragment := houseFrageFragment(site.Card)
	if fragment <= 0 {
		return 0, 0, false, errors.New("残页梦境奖励卡牌残页不存在")
	}
	mid, count, isCard := fragment, int64(0), false
	if rarity == 1 {
		count = int64(args.integer("n_count"))
		isCard = true
	} else {
		field := "r"
		if rarity == 3 {
			field = "sr"
		}
		if rarity == 4 {
			field = "ssr"
		}
		if rarity < 2 || rarity > 4 {
			return 0, 0, false, errors.New("残页梦境稀有度无效")
		}
		var cards, coins [][]int
		if json.Unmarshal(args[field+"_counts"], &cards) != nil || json.Unmarshal(args[field+"_coin_counts"], &coins) != nil {
			return 0, 0, false, errors.New("残页梦境奖励数量配置无效")
		}
		if site.Fixed > 0 {
			count = 1
			isCard = true
		} else {
			cw, ow := 0, 0
			for _, r := range cards {
				if len(r) != 2 {
					return 0, 0, false, errors.New("残页权重结构无效")
				}
				cw += r[1]
			}
			for _, r := range coins {
				if len(r) != 3 {
					return 0, 0, false, errors.New("代币权重结构无效")
				}
				ow += r[2]
			}
			chance := float64(cw) / float64(cw+ow)
			addition := 0.0
			if w.Up > 0 {
				addition = float64(args.integer("up_prop_value")) / 100
			}
			if rarity == 4 {
				bonus, e := houseFrageFacilitySSRBonus(p)
				if e != nil {
					return 0, 0, false, e
				}
				addition += bonus
			}
			// FF2A17AE先把up与ssr_add_prob相加，再乘残页基础概率。
			chance *= 1 + addition
			if chance > 1 {
				chance = 1
			}
			yes, e := activityChance(chance)
			if e != nil {
				return 0, 0, false, e
			}
			isCard = yes
			if yes {
				row, e := houseFrageWeight(cards)
				if e != nil {
					return 0, 0, false, e
				}
				count = int64(row[0])
			} else {
				row, e := houseFrageWeight(coins)
				if e != nil {
					return 0, 0, false, e
				}
				mid, count = row[0], int64(row[1])
			}
			if w.Up > 0 {
				w.Up--
			}
		}
	}
	if w.Double > 0 {
		count *= 2
		w.Double--
	}
	if isCard && rarity > 1 {
		add, e := houseFrageAdditions(p, level, rarity)
		if e != nil {
			return 0, 0, false, e
		}
		count += int64(add)
	}
	return mid, count, isCard, nil
}

// 原生碎片原石只生成待购买清单；用克隆进度展开同解释器，不能提前发给玩家。
func houseFrageRefreshStone(p *Progress, mid int, now time.Time) error {
	mat, ok := androidShop.Materials[mid]
	if !ok || mat.Type != 16 || mat.Target <= 0 {
		return errors.New("碎片原石模板不存在")
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	var preview Progress
	if e = json.Unmarshal(raw, &preview); e != nil {
		return e
	}
	box, e := grantNativeBonus(&preview, mat.Target, 1, p.AvatarLevel, now)
	if e != nil {
		return e
	}
	materials := box["materials"].(map[int]int64)
	if len(box["cards"].([]any)) > 0 || box["runes"] != nil {
		return errors.New("碎片原石包含非材料奖励，原生购买清单无法承载")
	}
	keys := []int{}
	for id, n := range materials {
		if n <= 0 {
			return errors.New("碎片原石待购数量无效")
		}
		keys = append(keys, id)
	}
	sort.Ints(keys)
	list := [][]int64{}
	for _, id := range keys {
		list = append(list, []int64{int64(id), materials[id]})
	}
	if len(list) == 0 {
		return errors.New("碎片原石待购清单为空")
	}
	w := ensureHouseFrage(p)
	w.Stone = HouseFrageStone{ID: mid, Time: now.Unix(), Materials: list}
	return nil
}
func houseFrageLanding(p *Progress, site *HouseFrageSite, level int, now time.Time) (map[string]any, int, error) {
	w := ensureHouseFrage(p)
	r := activityData("card_house", site.ID)
	box := emptyActivityBox()
	changes := box["materials"].(map[int]int64)
	cards := []string{}
	mid, count := 0, int64(0)
	args := activityData("card_house_params", w.ID)
	switch site.Prop {
	case 1:
		w.Double = args.integer("double_prop_count")
	case 2:
		w.Up = args.integer("up_prop_count")
	case 3:
		if e := grantNativeItem(p, 402, 1, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return nil, 0, e
		}
	}
	if site.Card > 0 {
		var isCard bool
		var e error
		mid, count, isCard, e = houseFrageCardBonus(p, site, level)
		if e != nil {
			return nil, 0, e
		}
		if isCard {
			if err := houseFragePolicyReward(w, activityData("cards", site.Card).integer("rarity")); err != nil {
				return nil, 0, err
			}
			w.Total++
			if activityData("cards", site.Card).integer("rarity") == 4 {
				w.SSR++
			}
		}
	} else if r.integer("site_type") == 5 {
		pair := r.ids("coin_info")
		if len(pair) != 2 {
			return nil, 0, errors.New("残页梦境金币格配置无效")
		}
		mid, count = pair[0], int64(pair[1])
		if w.Double > 0 {
			count *= 2
			w.Double--
		}
	} else if site.Material > 0 {
		mid, count = site.Material, 1
		if w.Double > 0 {
			count *= 2
			w.Double--
		}
	}
	if mid > 0 && count > 0 {
		if e := grantNativeItem(p, mid, count, p.AvatarLevel, now, changes, &cards, 0); e != nil {
			return nil, 0, e
		}
	}
	box["cards"] = cardListWire(p, cards)
	return box, site.Prop, nil
}
func houseFrageMove(p *Progress, value int, ctrl bool, level int, now time.Time) ([]any, error) {
	w := ensureHouseFrage(p)
	if len(w.Sites) == 0 || w.Sites[w.Pos] == nil || activityData("card_house", w.Pos).integer("site_type") == -1 {
		return nil, errors.New("残页梦境没有进行中的棋盘")
	}
	step := value
	mid := 401
	if ctrl {
		mid = 402
		if value < 1 || value > 6 || w.Slow > 0 {
			return nil, errors.New("遥控骰子步数或龟速状态无效")
		}
	} else {
		if value != 1 && value != 2 || w.Slow > 0 && value != 1 {
			return nil, errors.New("普通骰子模式无效")
		}
		step = 0
		for i := 0; i < value; i++ {
			n, e := activityAmount([]int64{1, 6})
			if e != nil {
				return nil, e
			}
			step += int(n)
		}
		if w.Slow > 0 {
			step = 1
			w.Slow--
		}
	}
	cost := p.Materials[mid]
	if cost.Count < 1 {
		return nil, errors.New("残页梦境骰子不足")
	}
	cost.Count--
	p.Materials[mid] = cost
	advanceAchievementAmount(p, 1008, nil, 1, now)
	pos, moved := w.Pos, 0
	for moved < step {
		r := activityData("card_house", pos)
		if r.integer("site_type") == -1 {
			break
		}
		next := r.integer("next_site_id")
		if moved == 0 && r.integer("site_type") == 4 {
			next = r.integer("branch_site_id")
		}
		if next == 0 {
			next = pos + 1
		}
		if w.Sites[next] == nil {
			return nil, errors.New("残页梦境路径配置断裂")
		}
		pos = next
		moved++
		if stone := activityData("card_house", pos).integer("frage_stone_id"); stone > 0 {
			if e := houseFrageRefreshStone(p, stone, now); e != nil {
				return nil, e
			}
		}
	}
	w.Pos = pos
	ended := activityData("card_house", pos).integer("site_type") == -1
	box := emptyActivityBox()
	prop := 0
	var e error
	if ended {
		box, e = grantActivityBonus(p, []int{activityData("card_house_params", w.ID).integer("end_bonus")}, now)
	} else {
		box, prop, e = houseFrageLanding(p, w.Sites[pos], level, now)
	}
	if e != nil {
		return nil, e
	}
	materials := box["materials"].(map[int]int64)
	// Android 869A57A0 原样转发奖励，1CBBBF23.on_roll_real_callback 对其调用
	// iteritems；必须发送实际材料字典。动画候选由 FF2A17AE 的
	// get_random_materials/get_end_materials 在客户端生成，不属于结算奖励。
	return []any{moved, pos, ended, materials, []any{prop, func() int {
		if prop == 1 {
			return w.Double
		}
		if prop == 2 {
			return w.Up
		}
		if prop == 3 {
			return 1
		}
		return 0
	}()}}, nil
}
func houseFragePrice(mid int) (int, int64, error) {
	var rows []struct {
		Key   []int `json:"键"`
		Value struct {
			Costs []int64 `json:"step_2_materials"`
		} `json:"值"`
	}
	if json.Unmarshal(androidActivities["frage_stone_prices"]["键值条目"], &rows) != nil {
		return 0, 0, errors.New("碎片原石价格目录无效")
	}
	rarity := activityData("materials", mid).integer("rarity")
	for _, key := range [][]int{{0, mid}, {rarity, 0}} {
		for _, row := range rows {
			if len(row.Key) == 2 && row.Key[0] == key[0] && row.Key[1] == key[1] {
				if len(row.Value.Costs) != 2 || row.Value.Costs[0] <= 0 || row.Value.Costs[1] < 0 {
					return 0, 0, errors.New("碎片原石价格无效")
				}
				return int(row.Value.Costs[0]), row.Value.Costs[1], nil
			}
		}
	}
	return mid, 1, nil
}
func (s *Service) houseFrageRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	lengths := map[string]int{"enter_house_frage": 1, "set_mark_card": 3, "house_frage_move": 2, "house_frage_ctrl_move": 2, "open_game_site": 2, "house_frage_use_prop": 2, "buy_frage_stone": 2, "get_house_board_game_reward": 0}
	if len(args) != lengths[method] {
		return nil, errors.New("残页梦境参数数量无效")
	}
	var cb, id, value int
	direct := method == "get_house_board_game_reward"
	if !direct && (json.Unmarshal(args[0], &cb) != nil || cb <= 0) {
		return nil, errors.New("残页梦境回调编号无效")
	}
	if len(args) > 1 && json.Unmarshal(args[1], &id) != nil {
		return nil, errors.New("残页梦境编号无效")
	}
	if len(args) > 2 && json.Unmarshal(args[2], &value) != nil {
		return nil, errors.New("残页梦境第二编号无效")
	}
	reply := []any{true, ""}
	bonus := map[int]int64{}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		level, e := houseFrageLevel(p)
		if e != nil {
			return e
		}
		w := ensureHouseFrage(p)
		refreshHouseFrageDaily(p, s.Now())
		switch method {
		case "enter_house_frage":
			if len(w.Sites) > 0 && activityData("card_house", w.Pos).integer("site_type") != -1 {
				return nil
			}
			return houseFrageRoll(p, level)
		case "set_mark_card":
			if id != 3 && id != 4 {
				return errors.New("残页梦境只支持精装或典藏祈愿")
			}
			if value == 0 {
				delete(w.Marks, id)
				return nil
			}
			r := activityData("cards", value)
			if len(r) == 0 || r.flag("disable") || r.integer("rarity") != id {
				return errors.New("残页梦境祈愿卡牌无效")
			}
			w.Marks[id] = value
			return nil
		case "house_frage_move", "house_frage_ctrl_move":
			reply, e = houseFrageMove(p, id, method == "house_frage_ctrl_move", level, s.Now())
			return e
		case "open_game_site":
			site := w.Sites[w.Pos]
			if site == nil || activityData("card_house", w.Pos).integer("site_type") != 7 || id < 0 || id >= len(site.Game) || containsInt(site.Opened, id) {
				return errors.New("残页梦境游戏选项无效或已开启")
			}
			count := int64(1)
			if w.Double > 0 {
				count = 2
				w.Double--
			}
			mid := site.Game[id]
			changes := map[int]int64{}
			cards := []string{}
			if e = grantNativeItem(p, mid, count, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
				return e
			}
			fragment := activityData("materials", mid)
			if fragment.integer("type") == 4 && fragment.integer("stype") == 1 {
				if err := houseFragePolicyReward(w, activityData("cards", fragment.integer("target_id")).integer("rarity")); err != nil {
					return err
				}
			}
			site.Opened = append(site.Opened, id)
			reply = []any{true, id, []int{mid}, count}
			return nil
		case "house_frage_use_prop":
			r := activityData("materials", id)
			if r.integer("type") != 4 || (r.integer("stype") != 16 && r.integer("stype") != 17) {
				return errors.New("残页梦境道具类型无效")
			}
			mat := p.Materials[id]
			if mat.Count <= 0 {
				return errors.New("残页梦境道具不足")
			}
			if r.integer("stype") == 16 {
				if len(w.Sites) == 0 || activityData("card_house", w.Pos).integer("site_type") == -1 {
					return errors.New("残页梦境当前棋盘未开始或已完成")
				}
				w.Slow = r.integer("target_id")
				if w.Slow <= 0 {
					return errors.New("龟速道具持续回合无效")
				}
			} else {
				if w.Stone.ID <= 0 {
					return errors.New("没有待刷新的碎片原石商店")
				}
				if e = houseFrageRefreshStone(p, w.Stone.ID, s.Now()); e != nil {
					return e
				}
			}
			mat.Count--
			p.Materials[id] = mat
			return nil
		case "buy_frage_stone":
			if w.Stone.ID <= 0 || id < 0 || id >= len(w.Stone.Materials) || len(w.Stone.Materials[id]) != 2 || w.Stone.Materials[id][1] <= 0 {
				return errors.New("碎片原石商品不存在或售罄")
			}
			mid := int(w.Stone.Materials[id][0])
			coin, cost, e := houseFragePrice(mid)
			if e != nil {
				return e
			}
			owned := p.Materials[coin]
			if owned.Count < cost {
				return errors.New("碎片原石商店货币不足")
			}
			changes := map[int]int64{}
			cards := []string{}
			if e = grantNativeItem(p, mid, 1, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
				return e
			}
			owned.Count -= cost
			p.Materials[coin] = owned
			w.Stone.Materials[id][1]--
			bonus[mid] = 1
			reply = []any{true, "", bonus}
			return nil
		case "get_house_board_game_reward":
			day := s.Now().In(time.FixedZone("北京时间", 28800)).Format("2006-01-02")
			if day <= w.RewardDay {
				return errors.New("骰子桌每日奖励已领取或时间回拨")
			}
			var effects [][]json.RawMessage
			_ = json.Unmarshal(activityData("facility_board_game", level)["upgrade_effect"], &effects)
			count := int64(0)
			for _, pair := range effects {
				if len(pair) != 2 {
					continue
				}
				var kind string
				_ = json.Unmarshal(pair[0], &kind)
				if kind == "11" {
					var gift []int64
					if json.Unmarshal(pair[1], &gift) != nil || len(gift) != 2 || gift[0] != 401 || gift[1] <= 0 {
						return errors.New("骰子桌每日骰子原生配置无效")
					}
					count += gift[1]
				}
			}
			if count <= 0 {
				return errors.New("骰子桌没有每日骰子奖励")
			}
			changes := map[int]int64{}
			cards := []string{}
			if e = grantNativeItem(p, 401, count, p.AvatarLevel, s.Now(), changes, &cards, 0); e != nil {
				return e
			}
			w.RewardDay = day
			f := p.Collection.Facilities[6]
			f.BoardReward = false
			p.Collection.Facilities[6] = f
			reply = []any{RetSuccess, []any{[]any{401, count}}}
			return nil
		}
		return errors.New("残页梦境业务未知")
	})
	if err != nil {
		switch method {
		case "house_frage_move", "house_frage_ctrl_move":
			p := c.SelectedAvatarUnsafe().Progress
			pos := 0
			if p.Activities.House != nil {
				pos = p.Activities.House.Pos
			}
			return []Push{nativeErrorPush(activityErrorCode(err)), Callback(cb, []any{0, pos, false, map[int]int64{}, []any{0, 0}})}, nil
		case "open_game_site":
			reply = []any{false, id, []any{}, 0}
		case "buy_frage_stone":
			reply = []any{false, err.Error(), map[int]int64{}}
		case "get_house_board_game_reward":
			return []Push{push("Avatar", "on_get_house_board_game_reward", activityErrorCode(err), []any{})}, nil
		default:
			reply = []any{false, err.Error()}
		}
		return []Push{Callback(cb, reply)}, nil
	}
	out := activityPushes(c, s.Now())
	out = append(out, materialManagerPush(c), cardMgrPush(c))
	if method == "get_house_board_game_reward" {
		out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)...)
	}
	if direct {
		return append(out, push("Avatar", "on_get_house_board_game_reward", reply...)), nil
	}
	return append(out, Callback(cb, reply)), nil
}

// 原生board_game_reward表示可领取；UTC+8日界只恢复资格，领取才实际入库存。
func refreshHouseFrageDaily(p *Progress, now time.Time) {
	if p.Collection == nil || p.Collection.Facilities[6].Level <= 0 {
		return
	}
	w := ensureHouseFrage(p)
	day := now.In(time.FixedZone("北京时间", 28800)).Format("2006-01-02")
	f := p.Collection.Facilities[6]
	f.BoardReward = day > w.RewardDay
	p.Collection.Facilities[6] = f
}
