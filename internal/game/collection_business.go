package game

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"hs-server/internal/mobileproto"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed collection_catalog.json
var collectionCatalogRaw []byte

type collectionFurniture struct {
	Type     int   `json:"type"`
	Comfort  int   `json:"satisfaction"`
	Bonus    int   `json:"collect_bonus_id"`
	Exchange int   `json:"exchange_flag"`
	Size     []int `json:"size"`
	Center   []int `json:"center"`
	Height   int   `json:"height"`
	Facility int   `json:"facility_id"`
	Theme    int   `json:"theme"`
	Material int   `json:"material_id"`
	Source   int   `json:"source"`
}
type collectionLevel struct {
	Level          int     `json:"level"`
	Material       int     `json:"material_id"`
	Count          int64   `json:"material_count"`
	Bonus          int     `json:"upgrade_bonus_id"`
	Produce        int     `json:"produce_material_id"`
	ProduceType    int     `json:"produce_type"`
	ProduceNum     float64 `json:"produce_material_num"`
	Unit           float64 `json:"unit_time"`
	Storage        float64 `json:"storage_max"`
	UnitBonusCount float64 `json:"unit_bonus_material_num"`
	UnitBonus      int     `json:"unit_bonus_id"`
	StageRewards   [][]int `json:"facility_card_reward"`
}
type collectionCatalog struct {
	Rooms map[int]struct {
		Grids      [][]float64 `json:"ground_grids"`
		Hollow     [][][]int   `json:"hollow_infos"`
		Extend     [][][]int   `json:"extend_infos"`
		Cards      int         `json:"house_card_num"`
		Limit      int         `json:"limit_furniture_count"`
		System     string      `json:"room_system_name"`
		CardProfit []float64   `json:"cards_num_profit"`
		Daily      int         `json:"house_daily_reward_id"`
	} `json:"rooms"`
	Facilities map[int]struct {
		Room  int    `json:"default_dormitory_id"`
		Sheet string `json:"related_data_sheet"`
	} `json:"facilities"`
	Levels    map[int]map[int]collectionLevel `json:"levels"`
	Furniture map[int]collectionFurniture     `json:"furniture"`
	Themes    map[int]struct {
		Bonus int `json:"handbook_bonus_id"`
	} `json:"themes"`
	Types map[int]struct {
		Kind  int `json:"furniture_type"`
		Count int `json:"satisfaction_counts"`
	} `json:"types"`
	Suits map[int]struct {
		Furniture []int `json:"suits_furniture_ids"`
		Comfort   int   `json:"suits_satisfaction"`
	} `json:"suits"`
	Comfort map[int]struct {
		Need  int `json:"satisfaction"`
		Extra int `json:"except_house_card_num"`
	} `json:"comfort"`
	Materials map[int]struct {
		Type   int   `json:"type"`
		Sub    int   `json:"stype"`
		Target int   `json:"target_id"`
		Limit  int64 `json:"limit_count"`
	} `json:"materials"`
	Bonuses map[int]struct {
		Fixed [][]int64         `json:"fixed_items"`
		Items []json.RawMessage `json:"random_items"`
		Runes []json.RawMessage `json:"random_runes"`
		Libs  []json.RawMessage `json:"random_item_libs"`
		Inner json.RawMessage   `json:"inner_bonus_id"`
	} `json:"bonuses"`
	Initial     [][]int64      `json:"initial"`
	Errors      map[string]int `json:"errors"`
	ExpInterval int64          `json:"exp_interval"`
	Cards       map[int]struct {
		Tags [][]int `json:"tag"`
	} `json:"cards"`
	Tags map[int]struct {
		Affect  int   `json:"affect_house"`
		Effects []int `json:"tag_effection"`
	} `json:"tags"`
	TagEffects map[int]struct {
		Condition int       `json:"tag_effect_condition"`
		Card      int       `json:"related_card_id"`
		Room      int       `json:"related_area"`
		Facility  int       `json:"related_facility"`
		Effect    string    `json:"tag_effect"`
		Data      []float64 `json:"tag_effect_data"`
	} `json:"tag_effects"`
	DailyRewards map[int]collectionDailyRule `json:"daily_rewards"`
}

var androidCollection = func() collectionCatalog {
	var c collectionCatalog
	if json.Unmarshal(collectionCatalogRaw, &c) != nil || len(c.Rooms) != 3 || len(c.Furniture) != 415 || c.ExpInterval != 5 {
		panic("Android收藏室目录无效")
	}
	return c
}()

type CollectionRoom struct {
	ID       int            `json:"dormitory_id"`
	Cards    map[int]int    `json:"cards"`
	Slots    map[int]int    `json:"dormitory_card_mgr"`
	LastExp  float64        `json:"last_receive_exp_time"`
	Velocity int64          `json:"exp_growth_velocity"`
	Limit    int64          `json:"exp_growth_limit"`
	First    bool           `json:"dormitory_first_work_flag"`
	Energy   map[string]any `json:"dormitory_energy"`
}
type CollectionFacility struct {
	ID              int          `json:"facility_id"`
	Level           int          `json:"level"`
	Upgrading       int64        `json:"upgrade_begin_time"`
	ProduceStart    float64      `json:"produce_start_time"`
	Keep            float64      `json:"produce_keep_num"`
	Rate            float64      `json:"produce_rate"`
	Storage         float64      `json:"storage_max"`
	BoardReward     bool         `json:"board_game_reward"`
	Recycle         int          `json:"card_recycle"`
	RecycleKeep     int          `json:"card_recycle_keep"`
	CardReward      map[int]bool `json:"card_facility_reward"`
	ComposeUnlocked bool         `json:"furniture_compose_unlock"`
}
type CollectionPlacement [7]int
type CollectionState struct {
	Version              int                                    `json:"version"`
	Rooms                map[int]CollectionRoom                 `json:"rooms"`
	Facilities           map[int]CollectionFacility             `json:"facilities"`
	Furniture            map[int]int64                          `json:"furniture"`
	Placements           []CollectionPlacement                  `json:"placements"`
	Wallpapers           map[int][]int                          `json:"wallpapers"`
	Handbook             map[int]int                            `json:"handbook"`
	Themes               map[int]int                            `json:"theme_handbook"`
	Dresses              map[int]int                            `json:"house_dresses"`
	RoomIndex            int                                    `json:"restroom_index"`
	AllowVisitors        bool                                   `json:"can_be_visited_by_stranger"`
	ShowSecretary        bool                                   `json:"show_others_secretary"`
	VisitRoom            int                                    `json:"room_id_for_visited"`
	DailyClaims          map[int]string                         `json:"daily_claims"`
	Wishlist             int                                    `json:"wishlist"`
	VisitOwner           string                                 `json:"visit_owner,omitempty"`
	VisitTime            float64                                `json:"visit_time,omitempty"`
	LikeDay              string                                 `json:"like_day,omitempty"`
	LikedOwners          map[string]bool                        `json:"liked_owners,omitempty"`
	ReceivedLikes        int64                                  `json:"received_likes,omitempty"`
	DailyReceivedLikes   map[int]int64                          `json:"daily_received_likes,omitempty"`
	ResidentDay          string                                 `json:"resident_day,omitempty"`
	ResidentWindows      map[int]map[int]CollectionResidentGift `json:"resident_windows,omitempty"`
	ResidentGiftCount    int64                                  `json:"resident_gift_count,omitempty"`
	TutorialAcceleration map[int]CollectionTutorialReceipt      `json:"tutorial_acceleration,omitempty"`
}

func newCollectionRoom(id int, now time.Time) CollectionRoom {
	return CollectionRoom{ID: id, Cards: map[int]int{}, Slots: map[int]int{}, LastExp: float64(now.Unix()), Velocity: 1, Limit: 1000, Energy: map[string]any{"start_flag": false, "last_update_time": float64(now.Unix()), "last_get_time": float64(0), "interval": 30, "base_cost": float64(0), "per_value": float64(0), "value": float64(0)}}
}
func ensureCollection(p *Progress, now time.Time) error {
	if p.Collection == nil {
		p.Collection = &CollectionState{Version: 1, RoomIndex: 1, VisitRoom: 1, AllowVisitors: true, ShowSecretary: true, Rooms: map[int]CollectionRoom{}, Furniture: map[int]int64{}}
		for _, r := range androidCollection.Initial {
			if len(r) == 2 {
				p.Collection.Furniture[int(r[0])] = r[1]
			}
		}
	}
	st := p.Collection
	if st.Rooms == nil {
		st.Rooms = map[int]CollectionRoom{}
	}
	if st.Facilities == nil {
		st.Facilities = map[int]CollectionFacility{}
	}
	if st.Furniture == nil {
		st.Furniture = map[int]int64{}
	}
	if st.Wallpapers == nil {
		st.Wallpapers = map[int][]int{}
	}
	if st.Handbook == nil {
		st.Handbook = map[int]int{}
	}
	if st.Themes == nil {
		st.Themes = map[int]int{}
	}
	if st.Dresses == nil {
		st.Dresses = map[int]int{}
	}
	if st.DailyClaims == nil {
		st.DailyClaims = map[int]string{}
	}
	if remainingPolicyEnabled() {
		if _, unlocked := p.UnlockSystems["house_dormitory3"]; unlocked {
			if _, owned := st.Rooms[3]; !owned {
				st.Rooms[3] = newCollectionRoom(3, now)
			}
		}
	}
	syncCollectionHandbook(p)
	reconcileCollectionAchievements(p, now)
	if err := collectionRefreshFacilities(p, now); err != nil {
		return err
	}
	return collectionRefreshEnergy(p, now)
}
func collectionFurnitureCount(p Progress, id int) int64 {
	if p.Collection == nil {
		return 0
	}
	r, ok := androidCollection.Furniture[id]
	if !ok {
		return 0
	}
	n := p.Collection.Furniture[id]
	if r.Type == 99 {
		return n
	}
	held := p.Materials[r.Material].Count
	if held > 0 && n <= math.MaxInt64-held {
		n += held
	}
	return n
}
func syncCollectionHandbook(p *Progress) {
	if p.Collection == nil {
		return
	}
	st := p.Collection
	for id := range androidCollection.Furniture {
		if collectionFurnitureCount(*p, id) > 0 && st.Handbook[id] == 0 {
			st.Handbook[id] = 1
		}
	}
	for id := range androidCollection.Themes {
		complete, required := true, 0
		for fid, r := range androidCollection.Furniture {
			if r.Theme != id || r.Type == 99 {
				continue
			}
			required++
			if st.Handbook[fid] == 0 {
				complete = false
			}
		}
		if required > 0 && complete && st.Themes[id] == 0 {
			st.Themes[id] = 1
		}
	}
}
func grantCollectionFurniture(p *Progress, id int, count int64, now time.Time) error {
	r, ok := androidCollection.Furniture[id]
	if !ok || r.Type == 99 || count <= 0 {
		return errors.New("家具奖励模板无效")
	}
	if e := ensureCollection(p, now); e != nil {
		return e
	}
	if p.Collection.Furniture[id] < 0 || p.Collection.Furniture[id] > math.MaxInt64-count {
		return errors.New("家具奖励数量溢出")
	}
	p.Collection.Furniture[id] += count
	syncCollectionHandbook(p)
	reconcileCollectionAchievements(p, now)
	return nil
}
func collectionProperties(p Progress, now time.Time) map[string]any {
	snapshot := CloneProgress(p)
	ensureCollection(&snapshot, now)
	st := snapshot.Collection
	rooms := map[int]CollectionRoom{}
	for id := range androidCollection.Rooms {
		rooms[id] = newCollectionRoom(id, time.Unix(0, 0))
	}
	for id, r := range st.Rooms {
		rooms[id] = r
	}
	unlocked := mobileproto.Map{}
	roomIDs := []int{}
	for id := range st.Rooms {
		roomIDs = append(roomIDs, id)
	}
	sort.Ints(roomIDs)
	for _, id := range roomIDs {
		unlocked = append(unlocked, mobileproto.Pair{Key: []any{1, id}, Value: true})
	}
	facilityIDs := []int{}
	for id := range st.Facilities {
		facilityIDs = append(facilityIDs, id)
	}
	sort.Ints(facilityIDs)
	for _, id := range facilityIDs {
		unlocked = append(unlocked, mobileproto.Pair{Key: []any{2, id}, Value: true})
	}
	used := map[int]int64{}
	grid := mobileproto.Map{}
	for _, r := range st.Placements {
		used[r[5]]++
		grid = append(grid, mobileproto.Pair{Key: []any{r[0], r[1], r[2], r[3], r[4]}, Value: []any{r[5], r[6]}})
	}
	for _, list := range st.Wallpapers {
		for _, id := range list {
			if id != 0 {
				used[id]++
			}
		}
	}
	furniture := map[int]any{}
	for id := range androidCollection.Furniture {
		if n := collectionFurnitureCount(snapshot, id); n > 0 {
			furniture[id] = []int64{n, used[id]}
		}
	}
	props := map[string]any{"house_unlock_state": activityWire(unlocked), "restroom_index": st.RoomIndex, "restroom_info": activityWire(rooms), "facility_info": activityWire(st.Facilities), "furniture_info": activityWire(furniture), "restroom_grid_info": activityWire(grid), "restroom_wallpapers": activityWire(st.Wallpapers), "furniture_handbook": activityWire(st.Handbook), "furniture_theme_handbook": activityWire(st.Themes), "up_furniture_material_id": st.Wishlist, "house_daily_reward": activityWire(collectionDailyAvailability(snapshot, now)), "can_be_visited_by_stranger": st.AllowVisitors, "show_others_secretary": st.ShowSecretary, "room_id_for_visited": st.VisitRoom}
	for name, value := range collectionSocialProperties(snapshot, now) {
		props[name] = value
	}
	props["house_daily_random_reward"] = activityWire(collectionResidentProperties(snapshot, now))
	return props
}
func collectionPushes(p Progress, now time.Time, ownership bool) []Push {
	props := collectionProperties(p, now)
	fields := []string{"house_unlock_state", "restroom_index", "restroom_info", "facility_info", "furniture_info", "restroom_grid_info", "restroom_wallpapers", "furniture_handbook", "furniture_theme_handbook", "up_furniture_material_id", "house_daily_reward", "can_be_visited_by_stranger", "show_others_secretary", "room_id_for_visited", "house_likes", "house_likes_map", "daily_house_likes", "house_daily_random_reward"}
	out := []Push{}
	for _, field := range fields {
		if field == "house_unlock_state" && !ownership {
			continue
		}
		out = append(out, push("Avatar", "client_prop_changed", []any{field, props[field]}))
	}
	return out
}
func collectionRoom(p *Progress, id int) (CollectionRoom, error) {
	r, ok := p.Collection.Rooms[id]
	if !ok {
		return r, runeReject("RET_HOUSE_FURNITURE_ROOM_ERROR", "收藏室房间尚未解锁")
	}
	return r, nil
}
func collectionSpend(p *Progress, id int, n int64) error {
	if n < 0 || id <= 0 {
		return errors.New("收藏室成本目录无效")
	}
	m := p.Materials[id]
	if m.Count < n {
		return runeReject("RET_MATERIAL_NOT_ENOUGH", "收藏室材料不足")
	}
	m.Count -= n
	if n > 0 {
		p.Materials[id] = m
	}
	return nil
}
func collectionComfort(p Progress, id int) int {
	groups := map[int][]int{}
	present := map[int]bool{}
	add := func(fid int) {
		r, ok := androidCollection.Furniture[fid]
		if ok {
			groups[r.Type] = append(groups[r.Type], r.Comfort)
			present[fid] = true
		}
	}
	for _, r := range p.Collection.Placements {
		if r[0] == id {
			add(r[5])
		}
	}
	for _, fid := range p.Collection.Wallpapers[id] {
		add(fid)
	}
	total := 0
	for kind, values := range groups {
		sort.Sort(sort.Reverse(sort.IntSlice(values)))
		limit := androidCollection.Types[kind].Count
		if len(values) < limit {
			limit = len(values)
		}
		for _, v := range values[:limit] {
			total += v
		}
	}
	for _, suit := range androidCollection.Suits {
		complete := len(suit.Furniture) > 0
		for _, fid := range suit.Furniture {
			complete = complete && present[fid]
		}
		if complete {
			total += suit.Comfort
		}
	}
	return total
}
func collectionCapacity(p Progress, id int) int {
	bonus := 0
	comfort := collectionComfort(p, id)
	for _, r := range androidCollection.Comfort {
		if comfort >= r.Need && r.Extra > bonus {
			bonus = r.Extra
		}
	}
	return androidCollection.Rooms[id].Cards + bonus
}
func collectionReceiveExp(p *Progress, id int, now time.Time) (int64, error) {
	r, err := collectionRoom(p, id)
	if err != nil {
		return 0, err
	}
	stamp := float64(now.Unix())
	if math.IsNaN(r.LastExp) || r.LastExp > stamp || r.Velocity < 0 || r.Limit < 0 {
		return 0, errors.New("收藏室好感度时钟或存档无效")
	}
	if len(r.Cards) == 0 {
		r.LastExp = stamp
		p.Collection.Rooms[id] = r
		return 0, nil
	}
	n := int64((stamp - r.LastExp) / float64(androidCollection.ExpInterval))
	if n > math.MaxInt64/maxInt64(1, r.Velocity) {
		return 0, errors.New("收藏室好感度增长溢出")
	}
	amount := n * r.Velocity
	if amount > r.Limit {
		amount = r.Limit
		r.LastExp = stamp
	} else {
		r.LastExp += float64(n * androidCollection.ExpInterval)
	}
	catalog, err := loadIntimacyCatalog()
	if err != nil {
		return 0, err
	}
	if p.Intimacy == nil {
		p.Intimacy = map[int]int{}
	}
	for cid := range r.Cards {
		old := int64(p.Intimacy[cid])
		if old < 0 || old > int64(catalog.MaxIntimacy) {
			return 0, errors.New("收藏室幻书好感度存档无效")
		}
		if amount > int64(catalog.MaxIntimacy)-old {
			p.Intimacy[cid] = catalog.MaxIntimacy
		} else {
			p.Intimacy[cid] += int(amount)
		}
	}
	p.Collection.Rooms[id] = r
	return amount, nil
}
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func deployCollectionCards(p *Progress, id int, infos [][]int, now time.Time) error {
	r, err := collectionRoom(p, id)
	if err != nil {
		return err
	}
	if len(infos) > collectionCapacity(*p, id) {
		return runeReject("RET_HOUSE_COMFORT_NOT_ENOUGH_TO_POS_CARD", "收藏室入住容量不足")
	}
	slots, cards := map[int]int{}, map[int]int{}
	dresses := map[int]int{}
	elsewhere := map[int]bool{}
	for rid, room := range p.Collection.Rooms {
		if rid != id {
			for cid := range room.Cards {
				elsewhere[cid] = true
			}
		}
	}
	for _, row := range infos {
		if len(row) != 3 || row[1] <= 0 || row[1] > collectionCapacity(*p, id) {
			return runeReject("RET_HOUSE_CHECK_CARD_ADD_ERROR", "收藏室入住参数无效")
		}
		cid, pos, dress := row[0], row[1], row[2]
		if !ownsCardID(*p, cid) {
			return runeReject("RET_CARD_NOT_EXIST", "入住幻书尚未拥有")
		}
		if elsewhere[cid] || cards[cid] != 0 || slots[pos] != 0 {
			return runeReject("RET_HOUSE_CARD_HAS_POSED", "幻书或入住位置重复")
		}
		if containsInt(androidCardAppearances[cid].Forbid, 3) {
			return runeReject("RET_CARD_FORBID_GROWUP", "此幻书禁止入住")
		}
		if !validOwnedDress(*p, cid, dress) {
			return runeReject("RET_CARD_DRESS_NOT_EXIST", "入住装帧尚未拥有")
		}
		slots[pos] = cid
		cards[cid] = 1
		dresses[cid] = dress
	}
	if _, err := collectionReceiveExp(p, id, now); err != nil {
		return err
	}
	r = p.Collection.Rooms[id]
	r.Cards = cards
	r.Slots = slots
	r.First = len(cards) > 0
	r.LastExp = float64(now.Unix())
	p.Collection.Rooms[id] = r
	for cid, dress := range dresses {
		p.Collection.Dresses[cid] = dress
	}
	return nil
}
func collectionAward(p *Progress, bid int, now time.Time, materials map[int]int64, cards *[]string) error {
	b, ok := androidCollection.Bonuses[bid]
	if !ok || len(b.Items)+len(b.Runes)+len(b.Libs) > 0 || (len(b.Inner) > 0 && string(b.Inner) != "null") {
		return errors.New("收藏室奖励结构未取证")
	}
	if len(b.Fixed) == 0 {
		return errors.New("收藏室奖励目录为空")
	}
	for _, row := range b.Fixed {
		if len(row) != 2 {
			return errors.New("收藏室奖励结构无效")
		}
		if err := grantShopItem(p, int(row[0]), row[1], now, materials, cards, 0); err != nil {
			return err
		}
	}
	return nil
}
func collectionClaimHandbook(p *Progress, method string, target int, now time.Time, materials map[int]int64, cards *[]string) error {
	syncCollectionHandbook(p)
	all := strings.HasPrefix(method, "receive_all_")
	theme := strings.Contains(method, "theme_handbook")
	ids := []int{}
	status := p.Collection.Handbook
	if theme {
		status = p.Collection.Themes
		if all {
			for id := range androidCollection.Themes {
				ids = append(ids, id)
			}
		} else {
			ids = []int{target}
		}
	} else if all {
		if _, ok := androidCollection.Themes[target]; !ok {
			return runeReject("RET_HOUSE_HANDBOOK_NOT_COLLECT", "家具主题不存在")
		}
		for id, r := range androidCollection.Furniture {
			if r.Theme == target && r.Type != 99 {
				ids = append(ids, id)
			}
		}
	} else {
		ids = []int{target}
	}
	sort.Ints(ids)
	for _, id := range ids {
		bid := 0
		if theme {
			bid = androidCollection.Themes[id].Bonus
		} else {
			bid = androidCollection.Furniture[id].Bonus
		}
		if all && (status[id] != 1 || bid == 0) {
			continue
		}
		if status[id] == 2 {
			label := "RET_HOUSE_HANDBOOK_BONUS_RECEIVED"
			if theme {
				label = "RET_HOUSE_HANDBOOK_THEME_BONUS_RECEIVED"
			}
			return runeReject(label, "家具手册奖励已领取")
		}
		if status[id] != 1 {
			label := "RET_HOUSE_HANDBOOK_NOT_COLLECT"
			if theme {
				label = "RET_HOUSE_HANDBOOK_THEME_NOT_COLLECT_ALL"
			}
			return runeReject(label, "家具手册收集尚未完成")
		}
		if bid == 0 {
			return errors.New("家具手册奖励不存在")
		}
		if err := collectionAward(p, bid, now, materials, cards); err != nil {
			return err
		}
		status[id] = 2
	}
	return nil
}

func (s *Service) collectionRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("收藏室操作需要玩家状态")
	}
	now := s.Now()
	materials := map[int]int64{}
	cards := []string{}
	replyID, cb, exp := 0, 0, int64(0)
	var box map[string]any
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, now); e != nil {
			return e
		}
		st := p.Collection
		integer := func(index int) (int, error) {
			var value int
			if index >= len(args) || json.Unmarshal(args[index], &value) != nil || value <= 0 {
				return 0, errors.New("收藏室编号参数无效")
			}
			return value, nil
		}
		switch method {
		case "player_enter_house":
			if len(args) != 0 {
				return errors.New("进入收藏室不接受参数")
			}
			if e := collectionGenerateResidentGifts(p, now); e != nil {
				return e
			}
		case "set_restroom_index":
			if len(args) != 1 {
				return errors.New("房间选择参数无效")
			}
			id, e := integer(0)
			if e != nil {
				return e
			}
			if _, known := androidCollection.Rooms[id]; !known {
				return errors.New("房间不存在")
			}
			st.RoomIndex = id
		case "unlock_dormitory", "unlock_facility":
			if len(args) != 1 {
				return errors.New("收藏室解锁参数无效")
			}
			mid, e := integer(0)
			if e != nil {
				return e
			}
			replyID = mid
			mat := androidCollection.Materials[mid]
			want := 7
			if method == "unlock_facility" {
				want = 6
			}
			if mat.Type != 4 || mat.Sub != want {
				return runeReject("RET_MATERIAL_WRONG", "不是合法收藏室钥匙")
			}
			if want == 7 {
				if _, ok := androidCollection.Rooms[mat.Target]; !ok {
					return errors.New("房间钥匙目标不存在")
				}
				if _, ok := st.Rooms[mat.Target]; ok {
					return runeReject("RET_HOUSE_ALREADY_UNLOCK", "房间已解锁")
				}
				if e := collectionSpend(p, mid, 1); e != nil {
					return e
				}
				st.Rooms[mat.Target] = newCollectionRoom(mat.Target, now)
			} else {
				f, ok := androidCollection.Facilities[mat.Target]
				if !ok {
					return errors.New("设施钥匙目标不存在")
				}
				if _, e := collectionRoom(p, f.Room); e != nil {
					return e
				}
				if _, owned := st.Facilities[mat.Target]; owned {
					return runeReject("RET_HOUSE_ALREADY_UNLOCK", "设施已解锁")
				}
				cost, ok := androidCollection.Levels[mat.Target][0]
				if !ok || cost.Material != mid {
					return errors.New("设施解锁成本目录不匹配")
				}
				if e := collectionSpend(p, mid, cost.Count); e != nil {
					return e
				}
				fstate := CollectionFacility{ID: mat.Target, Level: 1, ProduceStart: float64(now.Unix()), CardReward: map[int]bool{}}
				for _, stage := range androidCollection.Levels[mat.Target][1].StageRewards {
					if len(stage) == 2 {
						fstate.CardReward[stage[0]] = false
					}
				}
				st.Facilities[mat.Target] = fstate
				if bid := androidCollection.Levels[mat.Target][1].Bonus; bid != 0 {
					if e := collectionAward(p, bid, now, materials, &cards); e != nil {
						return e
					}
				}
			}
		case "upgrade_facility":
			if len(args) != 1 {
				return errors.New("设施升级参数无效")
			}
			id, e := integer(0)
			if e != nil {
				return e
			}
			replyID = id
			f, owned := st.Facilities[id]
			if !owned {
				return errors.New("设施尚未解锁")
			}
			rule, ok := androidCollection.Levels[id][f.Level+1]
			if !ok {
				return runeReject("RET_HOUSE_FACILITY_MAX_LEVEL", "设施已到最高等级")
			}
			if f.Upgrading != 0 {
				return runeReject("RET_HOUSE_FACILITY_UPGRADING", "设施正在升级")
			}
			if e := collectionSpend(p, rule.Material, rule.Count); e != nil {
				return e
			}
			f.Level++
			f.Upgrading = 0
			st.Facilities[id] = f
			if rule.Bonus != 0 {
				if e := collectionAward(p, rule.Bonus, now, materials, &cards); e != nil {
					return e
				}
			}
		case "dormitory_change_house_card", "set_restroom_girls":
			if len(args) != 2 {
				return errors.New("收藏室入住参数无效")
			}
			id, e := integer(0)
			if e != nil {
				return e
			}
			infos := [][]int{}
			if method == "set_restroom_girls" {
				var list intList
				if json.Unmarshal(args[1], &list) != nil {
					return errors.New("入住幻书列表无效")
				}
				for i, cid := range list {
					dress := st.Dresses[cid]
					if dress == 0 {
						dress = androidCardAppearances[cid].DefaultDress
					}
					infos = append(infos, []int{cid, i + 1, dress})
				}
			} else {
				if json.Unmarshal(args[1], &infos) != nil {
					return errors.New("入住位置与装帧列表无效")
				}
			}
			if e := deployCollectionCards(p, id, infos, now); e != nil {
				return e
			}
		case "house_card_change_dress_id":
			if len(args) != 2 {
				return errors.New("收藏室装帧参数无效")
			}
			cid, e := integer(0)
			if e != nil {
				return e
			}
			dress, e := integer(1)
			if e != nil {
				return e
			}
			replyID = cid
			if !ownsCardID(*p, cid) || !validOwnedDress(*p, cid, dress) {
				return runeReject("RET_CARD_DRESS_NOT_EXIST", "收藏室装帧尚未拥有")
			}
			st.Dresses[cid] = dress
		case "receive_restroom_exp":
			if len(args) != 2 {
				return errors.New("收藏室好感度领取参数无效")
			}
			var ok bool
			cb, ok = callbackArg(args)
			if !ok {
				return errors.New("收藏室好感度回调无效")
			}
			id, e := integer(1)
			if e != nil {
				return e
			}
			replyID = id
			exp, e = collectionReceiveExp(p, id, now)
			if e != nil {
				return e
			}
		case "update_visiting_setting":
			if len(args) != 3 {
				return errors.New("收藏室访客设置参数无效")
			}
			var allow, show bool
			if json.Unmarshal(args[0], &allow) != nil || json.Unmarshal(args[1], &show) != nil || string(args[0]) == "null" || string(args[1]) == "null" {
				return errors.New("收藏室访客设置必须是布尔值")
			}
			id, e := integer(2)
			if e != nil {
				return e
			}
			if _, e = collectionRoom(p, id); e != nil {
				return e
			}
			st.AllowVisitors = allow
			st.ShowSecretary = show
			st.VisitRoom = id
		case "receive_furniture_handbook_bonus", "receive_all_furniture_handbook_bonus", "receive_furniture_theme_handbook_bonus", "receive_all_furniture_theme_handbook_bonus":
			target := 0
			if method == "receive_all_furniture_theme_handbook_bonus" {
				if len(args) != 0 {
					return errors.New("全部主题手册领取不接受参数")
				}
			} else {
				if len(args) != 1 {
					return errors.New("家具手册领取参数无效")
				}
				var e error
				target, e = integer(0)
				if e != nil {
					return e
				}
			}
			if e := collectionClaimHandbook(p, method, target, now, materials, &cards); e != nil {
				return e
			}
		case "setup_furniture", "withdraw_furniture", "update_restroom_grid_info", "update_restroom_grid_wallpapers":
			if e := collectionLayoutRPC(p, method, args); e != nil {
				return e
			}
		case "exchange_furniture":
			if len(args) != 2 {
				return errors.New("家具兑换参数无效")
			}
			mid, e := integer(0)
			if e != nil {
				return e
			}
			fid, e := integer(1)
			if e != nil {
				return e
			}
			mat := androidCollection.Materials[mid]
			r, ok := androidCollection.Furniture[fid]
			if !ok || r.Exchange == 0 || mat.Type != 4 || mat.Sub != 27 {
				return runeReject("RET_MATERIAL_WRONG", "家具兑换材料或目标无效")
			}
			if e := collectionSpend(p, mid, 1); e != nil {
				return e
			}
			if e := grantCollectionFurniture(p, fid, 1, now); e != nil {
				return e
			}
		default:
			return errors.New("收藏室接口尚未实现")
		}
		syncCollectionHandbook(p)
		reconcileCollectionAchievements(p, now)
		box = map[string]any{"__custom_type": "box.box", "materials": materials, "cards": cardListWire(p, cards)}
		if e := collectionRefreshFacilities(p, now); e != nil {
			return e
		}
		return collectionRefreshEnergy(p, now)
	})
	code := RetSuccess
	if err != nil {
		var refusal *runeBusinessError
		if !errors.As(err, &refusal) {
			return nil, err
		}
		var known bool
		code, known = androidCollection.Errors[refusal.Name]
		if !known {
			return nil, fmt.Errorf("收藏室错误码不存在：%s", refusal.Name)
		}
		box = map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}}
	}
	out := []Push{}
	if err == nil {
		out = append(out, materialManagerPush(c), knowledgePush(c))
		out = append(out, collectionPushes(c.SelectedAvatarUnsafe().Progress, now, method == "unlock_dormitory" || method == "unlock_facility")...)
		out = append(out, push("Avatar", "client_prop_changed", []any{"card_common_mgr", cardCommonMgrPropertiesWithProgress(c.SelectedAvatarUnsafe().Progress)}))
	}
	switch method {
	case "player_enter_house":
		out = append(out, push("Avatar", "on_enter_house"))
	case "unlock_dormitory":
		out = append(out, push("Avatar", "on_unlock_dormitory", code, replyID))
	case "unlock_facility", "upgrade_facility":
		out = append(out, push("Avatar", "on_"+method, code, replyID, box))
	case "dormitory_change_house_card", "set_restroom_girls":
		out = append(out, push("Avatar", "on_dormitory_change_house_card", code))
	case "house_card_change_dress_id":
		dress := 0
		if len(args) == 2 {
			_ = json.Unmarshal(args[1], &dress)
		}
		out = append(out, push("Avatar", "on_house_card_change_dress_id", code, replyID, dress))
	case "receive_restroom_exp":
		if err != nil {
			return nil, err
		}
		out = append(out, Callback(cb, []any{replyID, exp}))
	case "receive_furniture_handbook_bonus", "receive_furniture_theme_handbook_bonus":
		out = append(out, push("Avatar", "on_"+method, code, box))
	case "receive_all_furniture_handbook_bonus", "receive_all_furniture_theme_handbook_bonus":
		if err != nil {
			return nil, err
		}
		out = append(out, push("Avatar", "on_"+method, box))
	case "exchange_furniture":
		out = append(out, push("Avatar", "on_exchange_furniture", code))
	case "setup_furniture", "withdraw_furniture", "update_restroom_grid_info", "update_restroom_grid_wallpapers":
		out = append(out, push("Avatar", "on_update_restroom_grid_info", code))
	}
	return out, nil
}

func collectionDress(p Progress, cardID int) int {
	if p.Collection != nil {
		if id := p.Collection.Dresses[cardID]; validOwnedDress(p, cardID, id) {
			return id
		}
	}
	return androidCardAppearances[cardID].DefaultDress
}
func collectionIntKey(id int) string { return strconv.Itoa(id) }
