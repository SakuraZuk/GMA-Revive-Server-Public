package game

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"time"
)

const CurrentRuneSchemaVersion = 1

type runeTemplate struct {
	Star             int       `json:"star"`
	Position         int       `json:"position"`
	BaseCount        int       `json:"base_attr_count"`
	BaseIDs          []int     `json:"base_attr_ids"`
	ExtraCount       int       `json:"extra_attr_count"`
	ExtraIDs         []int     `json:"extra_attr_ids"`
	ExtraProbability []float64 `json:"extra_attr_probability"`
}

// RuneSpec 为服务端奖励、管理入口使用的参数，不新增未取证的客户端 RPC。
type RuneSpec struct {
	Suit      int `json:"suit"`
	Position  int `json:"pos"`
	Star      int `json:"star"`
	Level     int `json:"level"`
	ExtraSuit int `json:"extra_suit"`
}

type QuarantinedRune struct {
	Record Rune   `json:"record"`
	Reason string `json:"reason"`
}

// RuneGrantReceipt 保存一次奖励的生成结果；分解后重试仍不重新发放。
type RuneGrantReceipt struct {
	Spec RuneSpec `json:"spec"`
	Rune Rune     `json:"rune"`
}

func GrantRuneOnce(p *Progress, receipt string, spec RuneSpec, now time.Time) (Rune, error) {
	if receipt == "" || len(receipt) > 128 {
		return Rune{}, errors.New("契印发放凭据无效")
	}
	if saved, ok := p.RuneGrantReceipts[receipt]; ok {
		if saved.Spec != spec {
			return Rune{}, errors.New("契印发放凭据与奖励内容冲突")
		}
		return saved.Rune, nil
	}
	r, err := GrantRune(p, spec, now)
	if err != nil {
		return Rune{}, err
	}
	if p.RuneGrantReceipts == nil {
		p.RuneGrantReceipts = map[string]RuneGrantReceipt{}
	}
	p.RuneGrantReceipts[receipt] = RuneGrantReceipt{Spec: spec, Rune: r}
	return r, nil
}

func runeTemplateFor(t runeTables, star, pos int) (runeTemplate, error) {
	for _, row := range t.Tables.Templates {
		if row.Star == star && row.Position == pos {
			return row, nil
		}
	}
	return runeTemplate{}, errors.New("契印星级或位置不在 Android 表中")
}

func runeSpecValid(spec RuneSpec, t runeTables) (bool, error) {
	if _, err := runeTemplateFor(t, spec.Star, spec.Position); err != nil {
		return false, err
	}
	if spec.Level < t.Tables.Bounds.Min || spec.Level > t.Tables.Bounds.Max {
		return false, errors.New("契印等级无效")
	}
	if spec.Position != 1 && spec.ExtraSuit != 0 {
		return false, errors.New("只有位置一允许额外套装")
	}
	locked := false
	for _, id := range []int{spec.Suit, spec.ExtraSuit} {
		if id == 0 && id == spec.ExtraSuit && spec.Suit != 0 {
			continue
		}
		found := false
		for _, row := range t.Tables.Suits {
			if row.ID == id {
				found = true
				for _, star := range row.AutoLockStars {
					locked = locked || star == spec.Star
				}
				break
			}
		}
		if !found {
			return false, errors.New("契印套装不在 Android 表中")
		}
	}
	levelFound := false
	for _, row := range t.Tables.Levels {
		if row.Star == spec.Star && row.Level == spec.Level {
			levelFound = true
			locked = locked || row.AutoLock != 0
			break
		}
	}
	if !levelFound {
		return false, errors.New("契印等级条目不存在")
	}
	return locked, nil
}

func validateRuneForEquip(r Rune, t runeTables) error {
	if !validObjectID(r.UUID) {
		return errors.New("契印 UUID 无效")
	}
	if _, err := runeSpecValid(RuneSpec{Suit: r.Suit, Position: r.Position, Star: r.Star, Level: r.Level, ExtraSuit: r.ExtraSuit}, t); err != nil {
		return err
	}
	template, _ := runeTemplateFor(t, r.Star, r.Position)
	entropy := &migrationRuneReader{seed: r.UUID}
	base, err := uniqueRuneAttrs(template.BaseIDs, template.BaseCount, r.BaseAttrs, t, entropy)
	if err != nil || !reflect.DeepEqual(base, r.BaseAttrs) {
		return errors.New("契印基础属性不符合模板或存在互斥")
	}
	library, err := runeLibrary(template, r.ExtraAttrsLib, t, entropy)
	if err != nil || len(library) != len(r.ExtraAttrsLib) {
		return errors.New("契印词条库不符合模板")
	}
	expected := map[int]bool{}
	for _, id := range library {
		expected[id] = true
	}
	for _, id := range r.ExtraAttrsLib {
		if !expected[id] {
			return errors.New("契印词条库包含重复或非法属性")
		}
		delete(expected, id)
	}
	maximum := len(template.ExtraProbability)
	if template.ExtraCount == 0 {
		maximum = 0
	}
	if r.ExtraAttrsCount < 0 || r.ExtraAttrsCount > maximum {
		return errors.New("契印词条数量不符合星级")
	}
	target := 0
	for _, row := range t.Tables.UnlockAttrs {
		if row.Unlock != 0 && row.Level <= r.Level {
			target++
		}
	}
	if target > r.ExtraAttrsCount {
		target = r.ExtraAttrsCount
	}
	if len(r.ExtraAttrs) != target {
		return errors.New("契印已解锁词条数量不正确")
	}
	lib := map[int]bool{}
	for _, id := range library {
		lib[id] = true
	}
	for _, id := range r.ExtraAttrs {
		if !lib[id] {
			return errors.New("契印属性不在词条库中")
		}
	}
	for id, factor := range r.ExtraAttrsFactor {
		if !lib[id] || factor <= 0 {
			return errors.New("契印权重因子无效")
		}
	}
	return nil
}

func chooseRuneWeights(weights map[int]int64, entropy io.Reader) (int, error) {
	ids := make([]int, 0, len(weights))
	var total int64
	for id, w := range weights {
		if w < 0 || total > math.MaxInt64-w {
			return 0, errors.New("契印抽选权重无效")
		}
		if w > 0 {
			total += w
			ids = append(ids, id)
		}
	}
	if total == 0 {
		return 0, errors.New("契印抽选候选为空")
	}
	sort.Ints(ids)
	n, err := rand.Int(entropy, big.NewInt(total))
	if err != nil {
		return 0, err
	}
	value := n.Int64()
	for _, id := range ids {
		value -= weights[id]
		if value < 0 {
			return id, nil
		}
	}
	return 0, errors.New("契印抽选失败")
}

func uniqueRuneAttrs(pool []int, count int, preferred []int, t runeTables, entropy io.Reader) ([]int, error) {
	return markedRuneAttrs(pool, count, preferred, nil, t, entropy)
}

// markedRuneAttrs 对照 Android rune_mgr.gen_attrs：按标记顺序抽取匹配属性，
// 删除已选属性及其互斥项；不匹配的标记不占名额，余量按原权重抽取。
func markedRuneAttrs(pool []int, count int, preferred []int, marks []string, t runeTables, entropy io.Reader) ([]int, error) {
	weights := map[int]int64{}
	mutex := map[int]int{}
	for _, id := range pool {
		found := false
		for _, row := range t.Tables.Attrs {
			if row.ID == id {
				found = true
				weights[id] = int64(row.Weight)
				if row.Mutex != nil {
					mutex[id] = *row.Mutex
				}
				break
			}
		}
		if !found {
			return nil, errors.New("契印模板引用不存在的属性")
		}
	}
	result := make([]int, 0, count)
	appendID := func(id int) { result = append(result, id); delete(weights, id); delete(weights, mutex[id]) }
	for _, id := range preferred {
		if _, ok := weights[id]; ok && len(result) < count {
			appendID(id)
		}
	}
	for _, mark := range marks {
		if len(result) >= count {
			break
		}
		candidates := map[int]int64{}
		for _, attr := range t.Tables.Attrs {
			if weight, ok := weights[attr.ID]; ok && strings.Contains(mark, attr.Name) {
				candidates[attr.ID] = weight
			}
		}
		if len(candidates) == 0 {
			continue
		}
		id, err := chooseRuneWeights(candidates, entropy)
		if err != nil {
			return nil, err
		}
		appendID(id)
	}
	for len(result) < count {
		id, err := chooseRuneWeights(weights, entropy)
		if err != nil {
			return nil, err
		}
		appendID(id)
	}
	return result, nil
}

func runeLibrary(template runeTemplate, preferred []int, t runeTables, entropy io.Reader) ([]int, error) {
	parents := map[int]int{}
	for _, id := range template.ExtraIDs {
		for _, attr := range t.Tables.Attrs {
			if attr.ID == id && attr.Bind != nil {
				parents[*attr.Bind] = id
			}
		}
	}
	rootsPreferred := make([]int, 0, len(preferred))
	for _, id := range preferred {
		if parent, ok := parents[id]; ok {
			id = parent
		}
		rootsPreferred = append(rootsPreferred, id)
	}
	roots, err := uniqueRuneAttrs(template.ExtraIDs, template.ExtraCount, rootsPreferred, t, entropy)
	if err != nil {
		return nil, err
	}
	result := append([]int{}, roots...)
	for _, id := range roots {
		for _, attr := range t.Tables.Attrs {
			if attr.ID == id && attr.Bind != nil {
				result = append(result, *attr.Bind)
			}
		}
	}
	return result, nil
}

func runeExtraCount(template runeTemplate, entropy io.Reader) (int, error) {
	if template.ExtraCount == 0 {
		return 0, nil
	}
	weights := map[int]int64{}
	for i, weight := range template.ExtraProbability {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight >= float64(math.MaxInt64) || math.Trunc(weight) != weight {
			return 0, errors.New("契印词条数量概率表无效")
		}
		weights[i+1] = int64(weight)
	}
	return chooseRuneWeights(weights, entropy)
}

// GenerateRune 所有数值来自 Android 表；UUID 使用系统熵源。
func GenerateRune(spec RuneSpec, now time.Time) (Rune, error) {
	return GenerateRuneWithMarks(spec, nil, now)
}

// GenerateRuneWithMarks 支持 Android runes_drops.mark_base_attrs 奖励规则。
// 标记不改变模板、词条容量或强化属性；不接受客户端自行指定标记。
func GenerateRuneWithMarks(spec RuneSpec, marks []string, now time.Time) (Rune, error) {
	t, err := loadRuneTables()
	if err != nil {
		return Rune{}, err
	}
	locked, err := runeSpecValid(spec, t)
	if err != nil {
		return Rune{}, err
	}
	template, _ := runeTemplateFor(t, spec.Star, spec.Position)
	r := Rune{Star: spec.Star, Position: spec.Position, Suit: spec.Suit, ExtraSuit: spec.ExtraSuit, Level: spec.Level, Locked: locked, CreateTime: float64(now.UnixNano()) / 1e9}
	if r.Position == 1 && r.ExtraSuit == 0 {
		r.ExtraSuit = r.Suit
	}
	var id [12]byte
	if _, err = io.ReadFull(rand.Reader, id[:]); err != nil {
		return Rune{}, err
	}
	r.UUID = hex.EncodeToString(id[:])
	if r.BaseAttrs, err = markedRuneAttrs(template.BaseIDs, template.BaseCount, nil, marks, t, rand.Reader); err != nil {
		return Rune{}, err
	}
	if r.ExtraAttrsLib, err = runeLibrary(template, nil, t, rand.Reader); err != nil {
		return Rune{}, err
	}
	if r.ExtraAttrsCount, err = runeExtraCount(template, rand.Reader); err != nil {
		return Rune{}, err
	}
	if err = fillRuneExtraAttrs(&r, t); err != nil {
		return Rune{}, err
	}
	return r, nil
}

// GrantRune 必须由调用者放入玩家行锁事务，避免容量检查与写入竞争。
func GrantRune(p *Progress, spec RuneSpec, now time.Time) (Rune, error) {
	return GrantRuneWithMarks(p, spec, nil, now)
}

// GrantRuneWithMarks 与 GrantRune 一样必须在玩家行锁事务内调用。
func GrantRuneWithMarks(p *Progress, spec RuneSpec, marks []string, now time.Time) (Rune, error) {
	t, err := loadRuneTables()
	if err != nil {
		return Rune{}, err
	}
	if len(p.Runes) >= t.Tables.Limits.Max {
		return Rune{}, errors.New("契印背包已满")
	}
	r, err := GenerateRuneWithMarks(spec, marks, now)
	if err != nil {
		return Rune{}, err
	}
	if _, exists := p.Runes[r.UUID]; exists {
		return Rune{}, errors.New("契印 UUID 冲突")
	}
	if p.Runes == nil {
		p.Runes = map[string]Rune{}
	}
	p.Runes[r.UUID] = r
	return r, nil
}

// migrationRuneReader 是仅用于一次性迁移的 UUID 派生字节流；不依赖登录时间。
type migrationRuneReader struct {
	seed    string
	counter uint64
	pending []byte
}

func (r *migrationRuneReader) Read(p []byte) (int, error) {
	for offset := 0; offset < len(p); {
		if len(r.pending) == 0 {
			var counter [8]byte
			binary.BigEndian.PutUint64(counter[:], r.counter)
			h := sha256.New()
			h.Write([]byte("hs-rune-migration-v1:" + r.seed))
			h.Write(counter[:])
			r.pending = h.Sum(nil)
			r.counter++
		}
		n := copy(p[offset:], r.pending)
		offset += n
		r.pending = r.pending[n:]
	}
	return len(p), nil
}

func normalizeLegacyRune(id string, r Rune, t runeTables) (Rune, error) {
	if !validObjectID(id) {
		return Rune{}, errors.New("旧契印 UUID 无效")
	}
	if r.UUID != "" && !strings.EqualFold(r.UUID, id) {
		return Rune{}, errors.New("旧契印 UUID 与容器键不一致")
	}
	r.UUID = strings.ToLower(id)
	// 没有取证 rune_id 到套装的映射，不能用编号猜测套装。
	if _, err := runeSpecValid(RuneSpec{Suit: r.Suit, Position: r.Position, Star: r.Star, Level: r.Level, ExtraSuit: r.ExtraSuit}, t); err != nil {
		return Rune{}, err
	}
	template, _ := runeTemplateFor(t, r.Star, r.Position)
	entropy := &migrationRuneReader{seed: r.UUID}
	var err error
	if r.BaseAttrs, err = uniqueRuneAttrs(template.BaseIDs, template.BaseCount, r.BaseAttrs, t, entropy); err != nil {
		return Rune{}, err
	}
	preferred := append(append([]int(nil), r.ExtraAttrsLib...), r.ExtraAttrs...)
	missingLibrary := len(r.ExtraAttrsLib) == 0
	if r.ExtraAttrsLib, err = runeLibrary(template, preferred, t, entropy); err != nil {
		return Rune{}, err
	}
	maximum := len(template.ExtraProbability)
	if template.ExtraCount == 0 {
		maximum = 0
	}
	if r.ExtraAttrsCount < 0 || r.ExtraAttrsCount > maximum || (missingLibrary && r.ExtraAttrsCount == 0 && maximum > 0) {
		if r.ExtraAttrsCount, err = runeExtraCount(template, entropy); err != nil {
			return Rune{}, err
		}
	}
	target := 0
	for _, row := range t.Tables.UnlockAttrs {
		if row.Unlock != 0 && row.Level <= r.Level {
			target++
		}
	}
	if target > r.ExtraAttrsCount {
		target = r.ExtraAttrsCount
	}
	lib := map[int]bool{}
	for _, attr := range r.ExtraAttrsLib {
		lib[attr] = true
	}
	extras := []int{}
	for _, attr := range r.ExtraAttrs {
		if lib[attr] && len(extras) < target {
			extras = append(extras, attr)
		}
	}
	r.ExtraAttrs = extras
	factors := map[int]int{}
	for attr, factor := range r.ExtraAttrsFactor {
		if lib[attr] && factor > 0 {
			factors[attr] = factor
		}
	}
	r.ExtraAttrsFactor = factors
	if err = fillRuneExtraAttrsWithReader(&r, t, entropy); err != nil {
		return Rune{}, err
	}
	if r.Position == 1 && r.ExtraSuit == 0 {
		r.ExtraSuit = r.Suit
	}
	if math.IsNaN(r.CreateTime) || math.IsInf(r.CreateTime, 0) || r.CreateTime < 0 {
		r.CreateTime = 0
	}
	return r, nil
}

// MigrateRunes 保留迁移前原件，隔离无法确定身份/配置的记录；只在玩家行锁下调用。
func MigrateRunes(p *Progress) error {
	if p.RuneSchemaVersion == CurrentRuneSchemaVersion {
		return nil
	}
	if p.RuneSchemaVersion > CurrentRuneSchemaVersion {
		return errors.New("契印存档版本高于服务端支持版本")
	}
	t, err := loadRuneTables()
	if err != nil {
		return err
	}
	rows := map[string]Rune{}
	originals := map[string]Rune{}
	quarantine := map[string]QuarantinedRune{}
	for id, row := range p.RuneMigrationOriginals {
		originals[id] = row
	}
	for id, row := range p.RuneQuarantine {
		quarantine[id] = row
	}
	keys := make([]string, 0, len(p.Runes))
	for id := range p.Runes {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	owned := map[string]bool{}
	for _, card := range p.Cards {
		owned[strings.ToLower(card.UUID)] = true
	}
	slots := map[string]bool{}
	for _, id := range keys {
		before := p.Runes[id]
		row, err := normalizeLegacyRune(id, before, t)
		if err == nil {
			if _, duplicate := rows[row.UUID]; duplicate {
				err = errors.New("旧契印包含重复 UUID")
			}
		}
		if err != nil {
			quarantine[id] = QuarantinedRune{Record: before, Reason: err.Error()}
			if _, ok := originals[id]; !ok {
				originals[id] = before
			}
			continue
		}
		row.CardUUID = strings.ToLower(row.CardUUID)
		slot := fmt.Sprintf("%s:%d", row.CardUUID, row.Position)
		if row.CardUUID != "" {
			if !owned[row.CardUUID] || slots[slot] {
				row.CardUUID = ""
			} else {
				slots[slot] = true
			}
		}
		rows[row.UUID] = row
		if !reflect.DeepEqual(before, row) || id != row.UUID {
			if _, ok := originals[id]; !ok {
				originals[id] = before
			}
		}
	}
	p.Runes, p.RuneMigrationOriginals, p.RuneQuarantine, p.RuneSchemaVersion = rows, originals, quarantine, CurrentRuneSchemaVersion
	return nil
}
