package game

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"hs-server/internal/mobileproto"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed activity_catalog.json
var activityCatalogRaw []byte

// 活动表和来源固定来自本项目Android；不使用参考项目的初始化或奖励数值。
var androidActivities = func() map[string]map[string]json.RawMessage {
	var c struct {
		Tables map[string]map[string]json.RawMessage `json:"表"`
	}
	if err := json.Unmarshal(activityCatalogRaw, &c); err != nil {
		panic(err)
	}
	if len(c.Tables["summer_node"]) != 288 || len(c.Tables["wangyan_dungeons"]) != 254 {
		panic("Android活动目录无效")
	}
	return c.Tables
}()

type activityRow map[string]json.RawMessage

func activityData(table string, id int) activityRow {
	var r activityRow
	_ = json.Unmarshal(androidActivities[table][strconv.Itoa(id)], &r)
	return r
}
func (r activityRow) integer(key string) int { var n int; _ = json.Unmarshal(r[key], &n); return n }
func (r activityRow) ids(key string) []int   { var n []int; _ = json.Unmarshal(r[key], &n); return n }
func (r activityRow) flag(key string) bool   { var n bool; _ = json.Unmarshal(r[key], &n); return n }
func (r activityRow) conditions(key string, p Progress, level int) error {
	var x [][][]json.RawMessage
	if len(r[key]) == 0 {
		return nil
	}
	if json.Unmarshal(r[key], &x) != nil {
		return errors.New("活动条件结构无效")
	}
	ok, err := shopConditions(x, p, level)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("活动前置条件未满足")
	}
	return nil
}

// ActivitySchedule 是显式运营配置，不能由客户端上传。无覆盖时沿用原生历史时间。
type ActivitySchedule struct {
	ID        int   `json:"activity_id"`
	Begin     int64 `json:"begin_time"`
	End       int64 `json:"end_time"`
	Enabled   bool  `json:"enabled"`
	Permanent bool  `json:"permanent"`
}

var configuredActivities atomic.Value

func SetActivitySchedules(rows []ActivitySchedule) error {
	if len(rows) > len(androidActivities["activity_type"]) {
		return errors.New("活动日程数量超限")
	}
	data := map[int]ActivitySchedule{}
	for _, r := range rows {
		if r.ID <= 0 || len(activityData("activity_type", r.ID)) == 0 || r.Begin <= 0 || (!r.Permanent && (r.End <= r.Begin || r.End-r.Begin > 366*86400)) || (r.Permanent && r.End != 0) {
			return errors.New("活动日程无效")
		}
		if _, ok := data[r.ID]; ok {
			return errors.New("活动日程编号重复")
		}
		data[r.ID] = r
	}
	configuredActivities.Store(data)
	return nil
}
func activitySchedule(id int) ActivitySchedule {
	if v := configuredActivities.Load(); v != nil {
		if r, ok := v.(map[int]ActivitySchedule)[id]; ok {
			return r
		}
	}
	r := activityData("activity_type", id)
	var begin, end []int64
	_ = json.Unmarshal(r["begin_time"], &begin)
	_ = json.Unmarshal(r["end_time"], &end)
	out := ActivitySchedule{ID: id, Enabled: r.integer("disable_activity") == 0}
	for _, n := range begin {
		out.Begin += n
	}
	for _, n := range end {
		out.End += n
	}
	return out
}

// MobileRPC编码器只直接识别[]any、map[string]any和mobileproto.Map；所有活动状态
// 在边界转换，保留原生整数键，避免结构体被默认编码为fmt.Sprint字符串。
func activityWire(value any) any {
	if value == nil {
		return nil
	}
	return activityWireReflect(reflect.ValueOf(value))
}
func activityWireReflect(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Type() == reflect.TypeOf(mobileproto.ObjectID{}) {
		return v.Interface()
	}
	if v.Type() == reflect.TypeOf(ObjectID("")) {
		return v.Interface()
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.Slice, reflect.Array:
		if v.Type() == reflect.TypeOf(mobileproto.Map{}) {
			out := mobileproto.Map{}
			for i := 0; i < v.Len(); i++ {
				pair := v.Index(i).Interface().(mobileproto.Pair)
				out = append(out, mobileproto.Pair{Key: activityWire(pair.Key), Value: activityWire(pair.Value)})
			}
			return out
		}
		out := []any{}
		for i := 0; i < v.Len(); i++ {
			out = append(out, activityWireReflect(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			out := map[string]any{}
			iter := v.MapRange()
			for iter.Next() {
				out[iter.Key().String()] = activityWireReflect(iter.Value())
			}
			return out
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].Int() < keys[j].Int() })
		out := mobileproto.Map{}
		for _, key := range keys {
			out = append(out, mobileproto.Pair{Key: activityWireReflect(key), Value: activityWireReflect(v.MapIndex(key))})
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" || field.Tag.Get("wire") == "-" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			out[name] = activityWireReflect(v.Field(i))
		}
		return out
	}
	return nil
}
func activityOpen(p Progress, id, level int, now time.Time) (int, error) {
	r := activityData("activity_type", id)
	w := activitySchedule(id)
	if len(r) == 0 || !w.Enabled || w.Begin <= 0 || (!w.Permanent && w.End <= w.Begin) {
		return 0, runeReject("RET_FAILED", "活动尚未配置开放日程")
	}
	if now.Unix() < w.Begin {
		return 0, runeReject("RET_FAILED", "活动尚未开始")
	}
	if !w.Permanent && now.Unix() > w.End {
		return 0, runeReject("RET_FAILED", "活动已结束")
	}
	if err := r.conditions("unlock_condition", p, level); err != nil {
		return 0, err
	}
	z := time.FixedZone("北京时间", 8*3600)
	a := time.Unix(w.Begin, 0).In(z)
	n := now.In(z)
	aa := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, z)
	nn := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, z)
	day := int(nn.Sub(aa)/(24*time.Hour)) + 1
	if w.Permanent {
		var begin, end []int64
		_ = json.Unmarshal(r["begin_time"], &begin)
		_ = json.Unmarshal(r["end_time"], &end)
		var b, e int64
		for _, v := range begin {
			b += v
		}
		for _, v := range end {
			e += v
		}
		if e > b && b > 0 {
			capDay := int((e-b)/86400) + 1
			if day > capDay {
				day = capDay
			}
		}
	}
	return day, nil
}

// 用户授权全部活动常驻；时间阶段沿用原表时长封顶，节点自身禁用和解锁条件仍校验。
func DefaultPermanentActivitySchedules(begin time.Time) []ActivitySchedule {
	ids := []int{}
	for key := range androidActivities["activity_type"] {
		id, _ := strconv.Atoi(key)
		ids = append(ids, id)
	}
	sort.Ints(ids)
	rows := make([]ActivitySchedule, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, ActivitySchedule{ID: id, Begin: begin.Unix(), Enabled: true, Permanent: true})
	}
	return rows
}
func activityForShop(shopID int) (int, bool) {
	ids := activityIDsForShop(shopID)
	if len(ids) == 0 {
		return 0, false
	}
	return ids[0], true
}
func activityIDsForShop(shopID int) []int {
	ids := []int{}
	for key := range androidActivities["activity_type"] {
		id, _ := strconv.Atoi(key)
		if containsInt(activityData("activity_type", id).ids("shop_ids"), shopID) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}
func activityShopAvailable(p Progress, shopID, level int, now time.Time) (bool, error) {
	ids := activityIDsForShop(shopID)
	if len(ids) == 0 {
		return false, nil
	}
	// 共用商店属于关联活动的并集；按ID稳定检查，不能随机挑到关闭的一项。
	var firstErr error
	opened := false
	for _, id := range ids {
		if _, err := activityOpen(p, id, level, now); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if activitySchedule(id).Permanent {
			return true, nil
		}
		opened = true
	}
	if opened {
		return false, nil
	}
	return false, firstErr
}

// 所有服务器活动状态在同一个avatar_progress JSONB玩家行锁事务中保存。
type ActivityState struct {
	Summer      *SummerState                        `json:"summer,omitempty"`
	Wangyan     *WangyanState                       `json:"wangyan,omitempty"`
	Mountain    *MountainState                      `json:"mountain,omitempty"`
	Nian        map[int]ActivityProgress            `json:"nian,omitempty"`
	NianDay     string                              `json:"nian_progress_day,omitempty" wire:"-"`
	NianHistory map[int]map[int64]NianScoreSnapshot `json:"nian_hour_history,omitempty" wire:"-"`
	Calendar    *ActivityCalendar                   `json:"rank_calendar,omitempty" wire:"-"`
	Footprint   *NianFootprintState                 `json:"north_wind_footprint,omitempty"`
	Exam        *ExamState                          `json:"exam,omitempty"`
	Miku        *MikuState                          `json:"miku,omitempty"`
	Cthulhu     *CthulhuState                       `json:"cthulhu,omitempty"`
	House       *HouseFrageState                    `json:"house_frage,omitempty"`
	LastTime    int64                               `json:"last_time,omitempty"`
}
type ActivityProgress struct {
	ID          int   `json:"dungeon_id"`
	Progress    int   `json:"current_progress"`
	Bonus       int   `json:"bonus_progress"`
	Actions     int   `json:"total_action"`
	MaxDamage   int64 `json:"max_damage,omitempty"`
	TotalDamage int64 `json:"total_damage,omitempty"`
	Cards       []any `json:"cards_info"`
	Ranked      bool  `json:"server_ranked,omitempty" wire:"-"`
	RankHard    int   `json:"server_rank_hard,omitempty" wire:"-"`
	RankDamage  int64 `json:"server_rank_damage,omitempty" wire:"-"`
	RankCards   []any `json:"server_rank_cards,omitempty" wire:"-"`
	RankAt      int64 `json:"server_rank_updated_at,omitempty" wire:"-"`
}
type ActivityBattleContext struct {
	ActivityID       int                          `json:"activity_id"`
	DungeonID        int                          `json:"dungeon_id"`
	MapID            int                          `json:"map_id,omitempty"`
	NodeID           int                          `json:"node_id,omitempty"`
	Tasks            []int                        `json:"tower_task_list,omitempty"`
	PreparedAt       int64                        `json:"prepared_at"`
	Resource         int                          `json:"resource,omitempty"`
	Layer            int                          `json:"layer,omitempty"`
	Trunk            int                          `json:"trunk,omitempty"`
	Branch           int                          `json:"branch,omitempty"`
	Cycle            int                          `json:"cycle,omitempty"`
	Place            string                       `json:"place,omitempty"`
	Spent            map[int]int64                `json:"spent_materials,omitempty"`
	Angry            bool                         `json:"angry,omitempty"`
	NativeSkills     [][6]int                     `json:"native_battle_skills,omitempty"`
	PaidPower        int64                        `json:"paid_power,omitempty"`
	Statistics       *ActivityBattleStatistics    `json:"native_statistics,omitempty"`
	RankCards        []any                        `json:"native_rank_cards,omitempty"`
	NativeBuffs      map[int][]ActivityNativeBuff `json:"native_buffs,omitempty"`
	NativeMFields    map[int][][3]int             `json:"native_magic_field_skills,omitempty"`
	EnemyLevelAdded  *int                         `json:"native_enemy_level_added,omitempty"`
	SummerClosedBeta bool                         `json:"summer_closed_beta,omitempty"`
	MaterialRates    map[int]float64              `json:"native_material_rates,omitempty"`
	MountainSeason   int                          `json:"mountain_season,omitempty"`
	LeagueProtect    *LeagueProtectAuthorization  `json:"league_protect,omitempty"`
}
type ActivityNativeBuff struct {
	ID           int     `json:"buff_id"`
	Property     float64 `json:"user_property"`
	NullProperty bool    `json:"native_null_property,omitempty"`
}

func activityProperties(p Progress, now time.Time) map[string]any {
	out := map[string]any{}
	if p.Activities.Summer != nil {
		out["summer_game"] = summerProperties(p.Activities.Summer)
	}
	if p.Activities.Wangyan != nil {
		out["wangyan_game"] = wangyanProperties(p.Activities.Wangyan, now)
	}
	if p.Activities.Mountain != nil {
		out["mountain_game"] = p.Activities.Mountain
	}
	if p.Activities.Nian != nil {
		out["monster_nian_dungeon"] = nianProperties(p.Activities.Nian)
	}
	if p.Activities.Footprint != nil {
		out["north_wind_footprint_game"] = p.Activities.Footprint
	}
	if p.Activities.Exam != nil {
		out["exam_game_info"] = examProperties(p.Activities.Exam)
		// 原生exam.exam_choose_info是题号->choose_info，不能把两个整数当成字典值。
		previous := map[int]any{}
		for id, choice := range p.Activities.Exam.Prev {
			correct := 0
			if choice == activityData("exam_prev_problem", id).integer("correct_id") {
				correct = 1
			}
			previous[id] = map[string]any{"correct_num": correct, "total_num": 1}
		}
		out["prev_exam_correct_info"] = previous
		out["storyline_bonus_dict"] = p.Activities.Exam.Storyline
	}
	if p.Activities.Miku != nil {
		for _, m := range p.Activities.Miku.Maps {
			m.Bonus = mikuReceiptBox(&p, m)
		}
		out["miku_game"] = mikuProperties(p.Activities.Miku)
	}
	if p.Activities.Cthulhu != nil {
		out["cthulhu_infos"] = p.Activities.Cthulhu.Maps
		out["cthulhu_title"] = p.Activities.Cthulhu.Title
	}
	if p.Activities.House != nil {
		for k, v := range houseFrageProperties(p) {
			out[k] = v
		}
	}
	for k, v := range out {
		out[k] = activityWire(v)
	}
	return out
}
func activityPushes(c *Connection, now time.Time) []Push {
	out := []Push{}
	for k, v := range activityProperties(c.SelectedAvatarUnsafe().Progress, now) {
		out = append(out, push("Avatar", "client_prop_changed", []any{k, v}))
	}
	return out
}

// Android battle_task_statistics(378524DA).on_bid_set从extra_info读取tower_task_list。
// 仅从持久化且已校验的会话生成任务；客户端task_ids不能直接变成胜利凭证。
func activityBattleExtra(bc *ActivityBattleContext, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range extra {
		out[k] = v
	}
	delete(out, "tower_task_list")
	delete(out, "add_battle_skills")
	delete(out, "change_attr_data")
	delete(out, "hs_summer_closed_beta")
	delete(out, "league_protect_card")
	if bc != nil {
		if bc.LeagueProtect != nil {
			for _, key := range []string{"enemy_level", "enemy_level_added", "enemy_skill_level", "enemy_skill_enhance_count", "enemy_hp_factor", "enemy_atk_factor", "enemy_defence_factor"} {
				delete(out, key)
			}
			for key, value := range leagueProtectBattleExtra(bc) {
				out[key] = value
			}
		}
		if bc.SummerClosedBeta {
			out["hs_summer_closed_beta"] = true
		}
		if bc.EnemyLevelAdded != nil {
			// 汪言只信服务端冻结的战力修正，不接受客户端覆盖原怪物数值。
			for _, key := range []string{"enemy_level", "enemy_level_added", "enemy_skill_level", "enemy_skill_enhance_count", "enemy_hp_factor", "enemy_atk_factor", "enemy_defence_factor"} {
				delete(out, key)
			}
			out["enemy_level_added"] = *bc.EnemyLevelAdded
		}
		out["tower_task_list"] = append([]int{}, bc.Tasks...)
		out["add_battle_skills"] = append([][6]int{}, bc.NativeSkills...)
		if len(bc.NativeBuffs)+len(bc.NativeMFields) > 0 {
			attrs := map[string]any{}
			buffs := map[int][]any{}
			for camp, entries := range bc.NativeBuffs {
				for _, entry := range entries {
					var property any = entry.Property
					if entry.NullProperty {
						property = nil
					}
					buffs[camp] = append(buffs[camp], []any{entry.ID, property})
				}
			}
			if len(buffs) > 0 {
				attrs["buff"] = buffs
			}
			if len(bc.NativeMFields) > 0 {
				attrs["mf_skill"] = bc.NativeMFields
			}
			out["change_attr_data"] = attrs
		}
	}
	return out
}
func activityErrorCode(err error) int {
	var e *runeBusinessError
	if errors.As(err, &e) {
		if c, ok := androidShop.Errors[e.Name]; ok {
			return c
		}
	}
	return androidShop.Errors["RET_FAILED"]
}
func emptyActivityBox() map[string]any {
	return map[string]any{"__custom_type": "box.box", "materials": map[int]int64{}, "cards": []any{}}
}

func activityWeighted(weights []int) (int, error) {
	total := int64(0)
	for _, w := range weights {
		if w < 0 || total > math.MaxInt64-int64(w) {
			return 0, errors.New("活动概率权重无效")
		}
		total += int64(w)
	}
	if total <= 0 {
		return 0, errors.New("活动概率总和为空")
	}
	n, e := rand.Int(rand.Reader, big.NewInt(total))
	if e != nil {
		return 0, e
	}
	v := n.Int64()
	for i, w := range weights {
		v -= int64(w)
		if v < 0 {
			return i, nil
		}
	}
	return 0, errors.New("活动概率选择失败")
}

// 所有活动共用Android D44DBBA7奖励解释器；奖励盒、材料、幻书及契印同事务。
func grantActivityBonus(p *Progress, ids []int, now time.Time, levels ...int) (map[string]any, error) {
	level := p.AvatarLevel
	if len(levels) > 0 {
		level = levels[0]
	}
	box := emptyActivityBox()
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		part, err := grantNativeBonus(p, id, 1, level, now)
		if err != nil {
			return nil, err
		}
		if err = mergeActivityBox(box, part); err != nil {
			return nil, err
		}
	}
	return box, nil
}
func activityChance(chance float64) (bool, error) {
	if math.IsNaN(chance) || math.IsInf(chance, 0) || chance < 0 || chance > 1 {
		return false, errors.New("活动概率参数无效")
	}
	if chance == 0 {
		return false, nil
	}
	if chance == 1 {
		return true, nil
	}
	n, e := rand.Int(rand.Reader, big.NewInt(1<<53))
	if e != nil {
		return false, e
	}
	return float64(n.Int64())/float64(1<<53) < chance, nil
}
func activityAmount(bounds []int64) (int64, error) {
	if len(bounds) < 1 || len(bounds) > 2 || bounds[0] < 0 {
		return 0, errors.New("活动奖励数量区间无效")
	}
	if len(bounds) == 1 {
		return bounds[0], nil
	}
	if bounds[1] < bounds[0] || bounds[1]-bounds[0] >= math.MaxInt64 {
		return 0, errors.New("活动奖励数量区间溢出")
	}
	n, e := rand.Int(rand.Reader, big.NewInt(bounds[1]-bounds[0]+1))
	if e != nil {
		return 0, e
	}
	return bounds[0] + n.Int64(), nil
}

// prepareActivityDungeon 必须只在建立新会话时调用；原会话重入和恢复不能重复扣资源。
func prepareActivityDungeon(p *Progress, dungeonID, level int, now time.Time) (*ActivityBattleContext, error) {
	if dungeonID >= 110001 && dungeonID <= 110003 {
		return prepareLeagueProtectDungeon(p, dungeonID, now)
	}
	r := activityData("dungeons", dungeonID)
	id := r.integer("dungeon_type")
	switch id {
	case 203, 206, 207, 208, 209, 211, 328:
	default:
		return nil, nil
	}
	day, err := activityOpen(*p, id, level, now)
	if err != nil {
		return nil, err
	}
	if p.Activities.LastTime > now.Unix() {
		return nil, errors.New("活动存档时间回拨")
	}
	if r.flag("_dungeon_disable") {
		return nil, errors.New("活动副本已停用")
	}
	if err := r.conditions("unlock_condition", *p, level); err != nil {
		return nil, err
	}
	if days := r.ids("activity_open_days"); len(days) > 0 && !containsInt(days, day) {
		return nil, errors.New("活动副本未到开放日")
	}

	bc := &ActivityBattleContext{ActivityID: id, DungeonID: dungeonID, PreparedAt: now.Unix(), Tasks: r.ids("tower_task_list")}
	switch id {
	case 207:
		err = prepareWangyanDungeon(p, bc, level, now)
	case 211:
		err = prepareSummerDungeon(p, bc, level, now)
	case 203:
		err = prepareMountainDungeon(p, bc, day, level, now)
	case 328:
		err = prepareNianDungeon(p, bc, day)
	case 209:
		err = prepareExamDungeon(p, bc, day, level, now)
	case 206:
		err = prepareCthulhuDungeon(p, bc)
	case 208:
		err = prepareMikuDungeon(p, bc, day, level, now)
	}
	if err != nil {
		return nil, err
	}
	spent, err := activityMaterialCosts(p, r["need_materials"])
	if err != nil {
		return nil, err
	}
	bc.Spent = spent
	p.Activities.LastTime = now.Unix()
	return bc, nil
}

func settleActivityDungeon(p *Progress, b *BattleSession, win bool, tasks []int, now time.Time) (map[string]any, error) {
	bc := b.ActivityContext
	if bc == nil {
		return nil, nil
	}
	if bc.DungeonID != b.DungeonID || bc.PreparedAt <= 0 {
		return nil, errors.New("活动战斗上下文无效")
	}
	if b.RewardGranted {
		return emptyActivityBox(), nil
	}
	if bc.ActivityID == 14 {
		return settleLeagueProtectDungeon(p, b, win, now)
	}
	seenTasks := map[int]bool{}
	for _, t := range tasks {
		if !containsInt(bc.Tasks, t) || seenTasks[t] {
			return nil, errors.New("活动结果包含未注入或重复的战斗任务")
		}
		seenTasks[t] = true
	}
	r := activityData("dungeons", b.DungeonID)
	bonuses := []int{}
	if win {
		if !containsInt(p.ClearedDungeons, b.DungeonID) {
			bonuses = append(bonuses, r.integer("first_bonus"))
		}
		bonuses = append(bonuses, r.integer("bonus"))
	} else {
		bonuses = append(bonuses, r.integer("fail_bonus"))
	}
	var box map[string]any
	var err error
	if bc.ActivityID == 206 {
		box, err = grantCthulhuBonus(p, bonuses, now)
	} else {
		box, err = grantActivityBonus(p, bonuses, now)
	}
	if err != nil {
		return nil, err
	}
	if win && bc.ActivityID == 211 {
		if err = applySummerMaterialBonus(p, box, now); err != nil {
			return nil, err
		}
	}
	if win && bc.ActivityID == 203 {
		if err = applyFrozenMountainMaterials(p, bc, box, now); err != nil {
			return nil, err
		}
	}
	if win && bc.ActivityID == 207 {
		if err = applyWangyanMaterialBonus(p, box, now); err != nil {
			return nil, err
		}
	}
	if win {
		switch bc.ActivityID {
		case 207:
			err = finishWangyanDungeon(p, bc, now)
		case 211:
			err = finishSummerDungeon(p, bc, now)
		case 203:
			err = finishMountainDungeon(p, bc, now)
		case 328:
			err = finishNianDungeon(p, bc, now)
		case 209:
			err = finishExamDungeon(p, bc, tasks, now)
		case 208:
			err = finishMikuDungeon(p, bc, tasks, now, box)
		case 206:
			var extraBox map[string]any
			extraBox, err = finishCthulhuDungeon(p, bc, now)
			if err == nil {
				err = mergeActivityBox(box, extraBox)
			}
		}
	}
	if !win && err == nil {
		if bc.ActivityID == 207 {
			if err = refundWangyanSupply(p, bc, now); err != nil {
				return nil, err
			}
		}
		returned := r.integer("return_power")
		// 原生 D81B4FE0 仅按 double 放大返还，未给出初音折算公式。
		// 以会话真实支付收据封顶，旧会话没有收据时不能额外获得体力。
		if bc.PaidPower < 0 || bc.PaidPower > math.MaxInt32 {
			return nil, errors.New("活动体力支付收据无效")
		}
		if int64(returned) > bc.PaidPower {
			returned = int(bc.PaidPower)
		}
		if returned < 0 || int64(returned) > math.MaxInt32-int64(p.currentPower(now)) {
			return nil, errors.New("活动失败返还体力无效或溢出")
		}
		if returned > 0 {
			p.settlePowerRecovery(now)
			p.Power.Value += returned
			box["materials"].(map[int]int64)[1] += int64(returned)
		}
		var pairs [][]int64
		if raw := r["return_materials"]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &pairs) != nil {
				return nil, errors.New("活动失败返还材料结构无效")
			}
		}
		for _, pair := range pairs {
			if len(pair) != 2 || pair[0] <= 0 || pair[1] < 0 || pair[1] > bc.Spent[int(pair[0])] {
				return nil, errors.New("活动失败返还超过已扣材料")
			}
			if pair[1] == 0 {
				continue
			}
			part := emptyActivityBox()
			changes := part["materials"].(map[int]int64)
			cards := []string{}
			if err = grantNativeItem(p, int(pair[0]), pair[1], p.AvatarLevel, now, changes, &cards, 0); err != nil {
				return nil, err
			}
			part["cards"] = cardListWire(p, cards)
			if err = mergeActivityBox(box, part); err != nil {
				return nil, err
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if bc.ActivityID == 328 {
		if err = recordNianBattleDamage(p, bc, now); err != nil {
			return nil, err
		}
	}
	b.RewardGranted = true
	return box, nil
}

func (s *Service) activityRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) (out []Push, err error) {
	defer func() {
		for i := range out {
			for j := range out[i].Args {
				out[i].Args[j] = activityWire(out[i].Args[j])
			}
		}
	}()
	if c.phase != Playing {
		return nil, errors.New("活动接口需要玩家登录")
	}
	switch method {
	case "query_rank_list", "query_mountain_sea_own_rank":
		return s.activityRankRPC(ctx, c, method, args)
	case "open_grid":
		return s.nianFootprintRPC(ctx, c, args)
	case "enter_wangyan_game", "wangyan_game_enter_map", "wangyan_game_enter_dungeon", "submit_wangyan_consign_task":
		return s.wangyanRPC(ctx, c, method, args)
	case "summer_game_enter_map", "summer_game_move_to", "summer_game_move_boat", "summer_game_enter_node", "summer_game_choose_item", "summer_game_clear_sp", "start_fishing", "check_fishing_result", "receive_fish_handbook_bonus", "receive_all_fish_handbook_bonus", "receive_summer_banner_bonus":
		return s.summerRPC(ctx, c, method, args)
	case "mountainsea_enter_dungeon", "change_guard_site_cards", "receive_mountain_dungeon_bonus", "receive_monster_nian_progress_bonus":
		return s.mountainNianRPC(ctx, c, method, args)
	case "prev_exam_choose_opt", "receive_exam_study_bonus_all", "receive_exam_study_bonus", "receive_exam_review_bonus", "receive_exam_test_bonus", "exam_answer_problem", "exam_answer_bonus", "receive_storyline_bonus":
		return s.examRPC(ctx, c, method, args)
	case "enter_miku_map", "activate_handbook_item", "set_miku_auto_path", "enter_story_dungeon", "enter_curse_abyss_dungeon", "enter_miku_node", "leave_miku_map", "receive_miku_task_bonus", "receive_miku_like_song_bonus", "receive_miku_surprise_bonus", "receive_miku_achv_bonus":
		return s.mikuRPC(ctx, c, method, args)
	case "refresh_cthulhu_activity", "cthulhu_visit_init_item", "cthulhu_visit_layer_init_item", "cthulhu_visit_layer_finish_item", "cthulhu_move", "cthulhu_ctrl_move", "cthulhu_giveup", "add_cthulhu_point", "cthulhu_item_check", "cthulhu_check_all_in", "set_cthulhu_title":
		return s.cthulhuRPC(ctx, c, method, args)
	case "enter_house_frage", "set_mark_card", "house_frage_move", "house_frage_ctrl_move", "open_game_site", "house_frage_use_prop", "buy_frage_stone", "get_house_board_game_reward":
		return s.houseFrageRPC(ctx, c, method, args)
	default:
		return nil, errors.New("活动原生业务规则尚未实现")
	}
}

// 只扣原生need_materials明确列出的库存费用；失败返还只允许已冻结的扣费。
func activityMaterialCosts(p *Progress, raw json.RawMessage) (map[int]int64, error) {
	costs := map[int]int64{}
	if len(raw) == 0 || string(raw) == "null" {
		return costs, nil
	}
	var pairs [][]int64
	if json.Unmarshal(raw, &pairs) != nil {
		return nil, errors.New("活动材料费用结构无效")
	}
	for _, v := range pairs {
		if len(v) != 2 || v[0] <= 0 || v[1] <= 0 || costs[int(v[0])] > math.MaxInt64-v[1] {
			return nil, errors.New("活动材料费用数量无效")
		}
		costs[int(v[0])] += v[1]
	}
	for id, n := range costs {
		m, ok := androidShop.Materials[id]
		if !ok || m.Type != 4 || p.Materials[id].Count < n {
			return nil, errors.New("活动费用材料无效或不足")
		}
	}
	for id, n := range costs {
		m := p.Materials[id]
		m.Count -= n
		p.Materials[id] = m
	}
	return costs, nil
}

// 登录同玩家事务初始化原生活动容器，不能把进入活动当作开放状态的唯一来源。
// 地图、事件、初始赠送和消耗仍由对应入口校验，登录不代替节点解锁。
func ensureActivityLogin(p *Progress, level int, now time.Time) error {
	refreshHouseFrageDaily(p, now)
	if _, e := activityOpen(*p, 327, level, now); e == nil {
		ensureNianFootprint(p)
	}
	if _, e := activityOpen(*p, 203, level, now); e == nil {
		ensureMountain(p)
	}
	if _, e := activityOpen(*p, 206, level, now); e == nil {
		ensureCthulhu(p)
	}
	if _, e := activityOpen(*p, 207, level, now); e == nil {
		w := ensureWangyan(p, now)
		if e = refreshWangyanSupply(w, now); e != nil {
			return e
		}
	}
	if _, e := activityOpen(*p, 208, level, now); e == nil {
		ensureMiku(p)
		reconcileMikuAchievements(p)
	}
	if _, e := activityOpen(*p, 209, level, now); e == nil {
		ensureExam(p)
	}
	if _, e := activityOpen(*p, 211, level, now); e == nil {
		if e = refreshSummerDaily(ensureSummer(p), now); e != nil {
			return e
		}
	}
	if _, e := activityOpen(*p, 328, level, now); e == nil {
		if e = refreshNianDaily(p, now); e != nil {
			return e
		}
	}
	// 委托候选必须在真实登录/日界事务中落库，初始属性投影不能代替持久生成。
	if remainingPolicyEnabled() {
		return ensureRemainingGameplay(p, now)
	}
	return nil
}

// 条件12必须来自实际结算历史；唯一通关集合不能充当重复通关次数。
func activityConditionCount(p Progress, dungeonType, subtype int) int {
	if dungeonType != 209 || p.Activities.Exam == nil {
		return 0
	}
	count := 0
	for _, id := range p.Activities.Exam.Study {
		r := activityData("dungeons", id)
		if r.integer("dungeon_type") == dungeonType && (subtype == 0 || containsInt(r.ids("dungeon_sub_types"), subtype)) {
			count++
		}
	}
	return count
}
