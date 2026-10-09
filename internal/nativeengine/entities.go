package nativeengine

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
)

// UnitSnapshot允许原生嵌套attrs与客户端展开属性并存。映射只依赖稳定身份；
// 当前坐标、生命、行动条和EID大小都不能用来猜测两个实体是否相同。
type UnitSnapshot map[string]any

type stableUnit struct {
	eid, category, creator            string
	camp, role, nativeType, mf, skill int
	hasType, summon, hasOrigin        bool
	origin                            [3]int
}

func identityInteger(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), int64(int(v)) == v
	case json.Number:
		n, err := strconv.ParseInt(string(v), 10, 64)
		return int(n), err == nil && int64(int(n)) == n
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return int(n), err == nil && int64(int(n)) == n
	case float64:
		// JSON普通解码得到float64。拒绝非整数和已经无法表示的整数。
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1<<53 || v != math.Trunc(v) {
			return 0, false
		}
		return int(v), float64(int(v)) == v
	}
	return 0, false
}

func identityEID(value any) string {
	if v, ok := value.(string); ok {
		return v
	}
	if n, ok := identityInteger(value); ok {
		return strconv.Itoa(n)
	}
	return ""
}

func identityFlag(value any) (bool, bool) {
	if v, ok := value.(bool); ok {
		return v, true
	}
	if _, isString := value.(string); isString {
		return false, false
	}
	if n, ok := identityInteger(value); ok && (n == 0 || n == 1) {
		return n == 1, true
	}
	return false, false
}

func cube(value any) ([3]int, bool) {
	var items []any
	switch v := value.(type) {
	case []any:
		items = v
	case []int:
		for _, n := range v {
			items = append(items, n)
		}
	case [3]int:
		return v, int64(v[0])+int64(v[1])+int64(v[2]) == 0
	default:
		return [3]int{}, false
	}
	if len(items) != 3 {
		return [3]int{}, false
	}
	var result [3]int
	for i, item := range items {
		if _, ok := item.(string); ok {
			return [3]int{}, false
		}
		n, ok := identityInteger(item)
		if !ok || n < -1000000 || n > 1000000 {
			return [3]int{}, false
		}
		result[i] = n
	}
	return result, int64(result[0])+int64(result[1])+int64(result[2]) == 0
}

func unitValue(raw UnitSnapshot, key string, fallback any) any {
	if value, exists := raw[key]; exists {
		return value
	}
	if attrs, ok := raw["attrs"].(map[string]any); ok {
		if value, exists := attrs[key]; exists {
			return value
		}
	}
	return fallback
}

func readStableUnit(raw UnitSnapshot, flip bool) (stableUnit, bool) {
	u := stableUnit{eid: identityEID(raw["eid"])}
	var ok bool
	u.role, ok = identityInteger(raw["role"])
	if !ok || u.role < 0 || u.eid == "" {
		return u, false
	}
	if v := unitValue(raw, "native_type", nil); v != nil {
		u.nativeType, u.hasType = identityInteger(v)
		if !u.hasType {
			return u, false
		}
	}
	kind, _ := raw["kind"].(string)
	support, ok := identityFlag(unitValue(raw, "is_support", unitValue(raw, "support", false)))
	if !ok {
		return u, false
	}
	u.category = "fighter"
	if kind == "field" || u.hasType && u.nativeType == 4 {
		u.category = "field"
	} else if support || kind == "support" {
		u.category = "support"
	}
	defaultCamp := map[string]int{"ally": 1, "hero": 1, "enemy": 2, "monster": 2, "field": 0}
	camp := unitValue(raw, "camp", unitValue(raw, "camp_id", nil))
	if camp == nil {
		u.camp, ok = defaultCamp[kind]
	} else {
		u.camp, ok = identityInteger(camp)
	}
	if !ok || u.camp < 0 || u.camp > 3 || u.category != "field" && u.camp != 1 && u.camp != 2 {
		return u, false
	}
	if flip && (u.camp == 1 || u.camp == 2) {
		u.camp = 3 - u.camp
	}
	mf := unitValue(raw, "mf_id", 0)
	if mf == nil {
		mf = 0
	}
	u.mf, ok = identityInteger(mf)
	if !ok || u.mf < 0 {
		return u, false
	}
	u.creator = identityEID(unitValue(raw, "create_entity_eid", nil))
	u.skill, _ = identityInteger(unitValue(raw, "create_skill_id", nil))
	u.origin, u.hasOrigin = cube(unitValue(raw, "origin_coord", nil))
	if v := unitValue(raw, "is_summon", nil); v != nil {
		u.summon, ok = identityFlag(v)
		if !ok {
			return u, false
		}
	} else {
		u.summon = unitValue(raw, "create_entity_eid", nil) != nil || unitValue(raw, "create_skill_id", nil) != nil || unitValue(raw, "origin_coord", nil) != nil
	}
	return u, true
}

func indexStableUnits(rows []UnitSnapshot, flip bool) map[string]stableUnit {
	indexed := map[string]stableUnit{}
	seen, duplicates := map[string]bool{}, map[string]bool{}
	for _, raw := range rows {
		eid := identityEID(raw["eid"])
		if eid != "" {
			if seen[eid] {
				duplicates[eid] = true
			}
			seen[eid] = true
		}
		if unit, ok := readStableUnit(raw, flip); ok {
			indexed[unit.eid] = unit
		}
	}
	for eid := range duplicates {
		delete(indexed, eid)
	}
	return indexed
}

func sameUnitBase(n, c stableUnit) bool {
	return n.camp == c.camp && n.role == c.role && n.category == c.category && n.mf == c.mf && n.summon == c.summon && (!n.hasType || !c.hasType || n.nativeType == c.nativeType)
}

type spawnIdentity struct {
	camp, role, nativeType, mf, skill int
	category, creator                 string
	origin                            [3]int
}

func spawnKey(u stableUnit, creator string, origin [3]int, originOK bool) (spawnIdentity, bool) {
	return spawnIdentity{u.camp, u.role, u.nativeType, u.mf, u.skill, u.category, creator, origin}, u.summon && u.hasType && creator != "" && u.skill > 0 && originOK
}

// BindEntities对齐新版作者的稳定映射规则。重复/缺失/歧义身份保持未映射，
// 召唤物必须匹配已绑定创造者、创造技能和固定棋盘下的出生原点。
func BindEntities(native, client []UnitSnapshot, clientIsLeft bool, existing map[string]string, frame *CoordinateFrame) map[string]string {
	natives, clients := indexStableUnits(native, false), indexStableUnits(client, !clientIsLeft)
	result, cached := map[string]string{}, map[string]string{}
	used, counts := map[string]bool{}, map[string]int{}
	for _, c := range existing {
		counts[c]++
	}
	for n, c := range existing {
		nu, nOK := natives[n]
		cu, cOK := clients[c]
		if nOK && cOK && counts[c] == 1 && sameUnitBase(nu, cu) {
			cached[n] = c
		}
	}
	bind := func(n, c string) { result[n], used[c] = c, true }
	for n, c := range cached {
		if !natives[n].summon {
			bind(n, c)
		}
	}
	bindUnique := func(candidates map[string][]string) bool {
		count := map[string]int{}
		for _, matches := range candidates {
			for _, c := range matches {
				count[c]++
			}
		}
		changed := false
		for n, matches := range candidates {
			if len(matches) == 1 && count[matches[0]] == 1 {
				bind(n, matches[0])
				changed = true
			}
		}
		return changed
	}
	candidates := map[string][]string{}
	for n, nu := range natives {
		if result[n] != "" || nu.summon {
			continue
		}
		for c, cu := range clients {
			if !used[c] && sameUnitBase(nu, cu) {
				candidates[n] = append(candidates[n], c)
			}
		}
	}
	bindUnique(candidates)
	clientOrigins := map[string][3]int{}
	if frame != nil {
		for c, cu := range clients {
			if cu.summon && cu.hasOrigin {
				origin, err := frame.ClientToNative(cu.origin)
				if err == nil {
					clientOrigins[c] = origin
				}
			}
		}
	}
	clientKey := func(c string) (spawnIdentity, bool) {
		origin, ok := clientOrigins[c]
		return spawnKey(clients[c], clients[c].creator, origin, ok)
	}
	for {
		changed := false
		for n, c := range cached {
			nu := natives[n]
			if result[n] != "" || used[c] || !nu.summon {
				continue
			}
			nk, nOK := spawnKey(nu, result[nu.creator], nu.origin, nu.hasOrigin)
			ck, cOK := clientKey(c)
			if nOK && cOK && nk == ck {
				bind(n, c)
				changed = true
			}
		}
		candidates = map[string][]string{}
		for n, nu := range natives {
			if result[n] != "" || !nu.summon {
				continue
			}
			nk, nOK := spawnKey(nu, result[nu.creator], nu.origin, nu.hasOrigin)
			if !nOK {
				continue
			}
			for c := range clients {
				ck, cOK := clientKey(c)
				if !used[c] && cOK && nk == ck {
					candidates[n] = append(candidates[n], c)
				}
			}
		}
		if !bindUnique(candidates) && !changed {
			break
		}
	}
	return result
}

// 固定棋盘变换只允许作者已经取证的平移或中心反射。左右阵营/地图ID
// 不能推出坐标中心；必须由至少两个不同初始战斗单位的一致对应证明。
type CoordinateFrame struct {
	Sign        int    `json:"sign"`
	Offset      [3]int `json:"offset"`
	AnchorCount int    `json:"anchor_count"`
}

func (f CoordinateFrame) valid() bool {
	return (f.Sign == 1 || f.Sign == -1) && f.AnchorCount >= 2 && int64(f.Offset[0])+int64(f.Offset[1])+int64(f.Offset[2]) == 0
}

func (f CoordinateFrame) ClientToNative(coord [3]int) ([3]int, error) {
	if !f.valid() || int64(coord[0])+int64(coord[1])+int64(coord[2]) != 0 {
		return [3]int{}, errors.New("PVP棋盘变换或六角坐标无效")
	}
	var result [3]int
	for i, v := range coord {
		result[i] = f.Sign*v + f.Offset[i]
	}
	return result, nil
}

func (f CoordinateFrame) NativeToClient(coord [3]int) ([3]int, error) {
	if !f.valid() || int64(coord[0])+int64(coord[1])+int64(coord[2]) != 0 {
		return [3]int{}, errors.New("PVP棋盘变换或六角坐标无效")
	}
	var result [3]int
	for i, v := range coord {
		result[i] = f.Sign * (v - f.Offset[i])
	}
	return result, nil
}

func coordinateUnits(rows []UnitSnapshot) (map[string]UnitSnapshot, error) {
	units := map[string]UnitSnapshot{}
	for _, raw := range rows {
		if raw["eid"] == nil {
			continue
		}
		eid := identityEID(raw["eid"])
		if _, exists := units[eid]; exists {
			return nil, errors.New("PVP坐标快照中有重复EID")
		}
		units[eid] = raw
	}
	return units, nil
}

func fightingAnchor(raw UnitSnapshot) bool {
	kind, _ := raw["kind"].(string)
	typ, _ := identityInteger(raw["native_type"])
	support, _ := identityFlag(unitValue(raw, "support", false))
	if kind == "field" || kind == "support" || typ == 4 || support {
		return false
	}
	camp, _ := identityInteger(unitValue(raw, "camp", unitValue(raw, "camp_id", nil)))
	return kind == "ally" || kind == "enemy" || kind == "hero" || kind == "monster" || camp == 1 || camp == 2
}

func DeriveCoordinateFrame(native, client []UnitSnapshot, mapping map[string]string) (*CoordinateFrame, error) {
	natives, err := coordinateUnits(native)
	if err != nil {
		return nil, err
	}
	clients, err := coordinateUnits(client)
	if err != nil {
		return nil, err
	}
	var pairs [][2][3]int
	seenClient, nativeCells, clientCells := map[string]bool{}, map[[3]int]bool{}, map[[3]int]bool{}
	for n, c := range mapping {
		nu, nOK := natives[n]
		cu, cOK := clients[c]
		if !nOK || !cOK {
			return nil, errors.New("PVP坐标对应单位不在快照中")
		}
		if fightingAnchor(nu) != fightingAnchor(cu) {
			return nil, errors.New("PVP棋盘锚点的单位类别不一致")
		}
		if !fightingAnchor(nu) {
			continue
		}
		nRole, nOK := identityInteger(nu["role"])
		cRole, cOK := identityInteger(cu["role"])
		if seenClient[c] || !nOK || !cOK || nRole != cRole {
			return nil, errors.New("PVP棋盘锚点重复或角色不一致")
		}
		seenClient[c] = true
		nHex, nOK := cube(nu["hex"])
		cHex, cOK := cube(cu["hex"])
		if !nOK || !cOK {
			return nil, errors.New("PVP棋盘锚点六角坐标无效")
		}
		pairs = append(pairs, [2][3]int{nHex, cHex})
		nativeCells[nHex], clientCells[cHex] = true, true
	}
	if len(pairs) < 2 || len(nativeCells) < 2 || len(clientCells) < 2 {
		return nil, errors.New("PVP棋盘映射需要两个不同初始战斗锚点")
	}
	var candidate *CoordinateFrame
	for _, sign := range []int{1, -1} {
		f := &CoordinateFrame{Sign: sign, AnchorCount: len(pairs)}
		for i := range f.Offset {
			f.Offset[i] = pairs[0][0][i] - sign*pairs[0][1][i]
		}
		consistent := true
		for _, pair := range pairs {
			for i := range f.Offset {
				consistent = consistent && pair[0][i]-sign*pair[1][i] == f.Offset[i]
			}
		}
		if consistent {
			if candidate != nil {
				return nil, errors.New("PVP棋盘变换存在歧义")
			}
			candidate = f
		}
	}
	if candidate == nil {
		return nil, errors.New("PVP初始快照不能证明一个一致棋盘变换")
	}
	return candidate, nil
}
