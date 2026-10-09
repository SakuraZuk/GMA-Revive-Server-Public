package game

import (
	"encoding/json"
	"errors"
)

type collectionCell struct{ Room, Plane, Layer, X, Y int }

func collectionFurnitureKind(id int) int {
	r, ok := androidCollection.Furniture[id]
	if !ok {
		return 0
	}
	return androidCollection.Types[r.Type].Kind
}
func collectionCellExists(room, plane, x, y int) bool {
	r, ok := androidCollection.Rooms[room]
	if !ok || plane < 0 || plane >= len(r.Grids) || len(r.Grids[plane]) != 5 {
		return false
	}
	g := r.Grids[plane]
	inside := x >= 0 && y >= 0 && x < int(g[3]) && y < int(g[4])
	if plane < len(r.Extend) {
		for _, rect := range r.Extend[plane] {
			if collectionInRect(x, y, rect) {
				inside = true
			}
		}
	}
	return inside
}
func collectionInRect(x, y int, rect []int) bool {
	return len(rect) == 4 && x >= rect[0] && y >= rect[1] && x < rect[0]+rect[2] && y < rect[1]+rect[3]
}
func collectionValidCell(room, plane, x, y int) bool {
	if !collectionCellExists(room, plane, x, y) {
		return false
	}
	r := androidCollection.Rooms[room]
	if plane < len(r.Hollow) {
		for _, rect := range r.Hollow[plane] {
			if collectionInRect(x, y, rect) {
				return false
			}
		}
	}
	return true
}
func collectionFootprint(row CollectionPlacement) ([][2]int, error) {
	f := androidCollection.Furniture[row[5]]
	if len(f.Size) != 2 || len(f.Center) != 2 || f.Size[0] <= 0 || f.Size[1] <= 0 || f.Size[0] > 120 || f.Size[1] > 120 {
		return nil, errors.New("家具尺寸目录无效")
	}
	cells := [][2]int{}
	for x := 0; x < f.Size[0]; x++ {
		for y := 0; y < f.Size[1]; y++ {
			dx, dy := x-f.Center[0]+1, y-f.Center[1]+1
			switch row[6] {
			case 0:
				cells = append(cells, [2]int{row[3] + dx, row[4] + dy})
			case 1:
				cells = append(cells, [2]int{row[3] - dy, row[4] + dx})
			case 2:
				cells = append(cells, [2]int{row[3] - dx, row[4] - dy})
			case 3:
				cells = append(cells, [2]int{row[3] + dy, row[4] - dx})
			default:
				return nil, errors.New("家具朝向无效")
			}
		}
	}
	return cells, nil
}
func validateCollectionLayout(p Progress, rows []CollectionPlacement, walls map[int][]int) error {
	if len(rows) > 3*120 {
		return runeReject("RET_HOUSE_PLACE_TOO_MUCH_FURNITURES", "收藏室家具数量超过限制")
	}
	occupied := map[collectionCell]bool{}
	volumes := map[collectionCell]int{}
	keys := map[[5]int]bool{}
	used := map[int]int64{}
	counts := map[int]int{}
	merged := map[int]map[int]bool{}
	for _, row := range rows {
		room, plane, layer, x, y, id := row[0], row[1], row[2], row[3], row[4], row[5]
		if _, ok := p.Collection.Rooms[room]; !ok {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_ERROR", "布置房间尚未解锁")
		}
		r, ok := androidCollection.Furniture[id]
		if !ok {
			return runeReject("RET_HOUSE_FURNITURE_COUNT_ERROR", "家具不存在")
		}
		kind := collectionFurnitureKind(id)
		if plane < 0 || plane > 2 || layer < 1 || layer > 2 || x < -100 || x > 100 || y < -100 || y > 100 || containsInt([]int{1, 2, 7, 8, 100}, kind) {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "家具布置位置无效")
		}
		wanted := 2
		if kind == 3 || kind == 6 {
			wanted = 1
		}
		if layer != wanted || ((kind == 5 || kind == 6) && plane == 0) || ((kind == 3 || kind == 4 || kind == 99) && plane != 0) {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "家具所在平面或占用层无效")
		}
		if kind == 99 && androidCollection.Facilities[r.Facility].Room != room {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_ERROR", "设施家具所在房间无效")
		}
		key := [5]int{room, plane, layer, x, y}
		if keys[key] {
			return runeReject("RET_HOUSE_FURNITURE_GRID_OCCUPIED", "家具定位格重复")
		}
		keys[key] = true
		cells, e := collectionFootprint(row)
		if e != nil {
			return e
		}
		for _, xy := range cells {
			cell := collectionCell{room, plane, layer, xy[0], xy[1]}
			if !collectionValidCell(room, plane, xy[0], xy[1]) {
				return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "家具超出房间有效范围")
			}
			if occupied[cell] {
				return runeReject("RET_HOUSE_FURNITURE_GRID_OCCUPIED", "家具占用格重叠")
			}
			occupied[cell] = true
			if layer == 2 {
				volumes[collectionCell{room, plane, 2, xy[0], xy[1]}] = r.Height
			}
		}
		used[id]++
		if kind == 4 {
			counts[room]++
		} else if containsInt([]int{5, 3, 6}, kind) {
			if merged[room] == nil {
				merged[room] = map[int]bool{}
			}
			merged[room][id] = true
		}
		if counts[room]+len(merged[room]) > androidCollection.Rooms[room].Limit {
			return runeReject("RET_HOUSE_PLACE_TOO_MUCH_FURNITURES", "房间家具数量达到上限")
		}
	}
	// 根据原生ground_grids及0.35网格投影跨平面体积，墙饰与地面家具也须互相避让。
	for _, row := range rows {
		if row[2] != 2 {
			continue
		}
		cells, _ := collectionFootprint(row)
		if e := collectionCheckVolumes(row, cells, volumes); e != nil {
			return e
		}
	}
	for room, list := range walls {
		if _, ok := p.Collection.Rooms[room]; !ok {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_ERROR", "墙纸房间尚未解锁")
		}
		if len(list) != 4 {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "墙纸需要地板、墙、围栏及壁炉四槽")
		}
		for slot, id := range list {
			if id == 0 {
				continue
			}
			if id < 0 || collectionFurnitureKind(id) != []int{1, 2, 8, 7}[slot] {
				return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "墙纸类型与槽位不匹配")
			}
			used[id]++
		}
	}
	for id, n := range used {
		if collectionFurnitureCount(p, id) < n {
			return runeReject("RET_HOUSE_FURNITURE_COUNT_NOT_ENOUGH", "家具库存不足，拒绝客户端上传总量")
		}
	}
	return nil
}
func collectionCheckVolumes(row CollectionPlacement, cells [][2]int, volumes map[collectionCell]int) error {
	r := androidCollection.Rooms[row[0]]
	if len(r.Grids) != 3 {
		return errors.New("房间平面目录无效")
	}
	g, a, b := r.Grids[0], r.Grids[1], r.Grids[2]
	if len(g) != 5 || len(a) != 5 || len(b) != 5 {
		return errors.New("房间平面坐标无效")
	}
	gx, gy, gz, gh := g[0]-g[3]*0.35/2, g[1], g[2]-g[4]*0.35/2, int(g[4])
	ax, ay, az, aw := a[0], a[1]-a[4]*0.35/2, a[2]-a[3]*0.35/2, int(a[3])
	bx, by, bz := b[0]-b[3]*0.35/2, b[1]-b[4]*0.35/2, b[2]
	minx, maxy := mathGridMax(), -mathGridMax()
	xs, ys := map[int]bool{}, map[int]bool{}
	for _, xy := range cells {
		xs[xy[0]] = true
		ys[xy[1]] = true
		if xy[0] < minx {
			minx = xy[0]
		}
		if xy[1] > maxy {
			maxy = xy[1]
		}
	}
	height := androidCollection.Furniture[row[5]].Height
	check := func(plane, x, y, distance int, invalid bool) error {
		if invalid && distance < 1 && collectionCellExists(row[0], plane, x, y) && !collectionValidCell(row[0], plane, x, y) {
			return runeReject("RET_HOUSE_FURNITURE_ROOM_AREA_ERROR", "家具体积穿过房间空洞")
		}
		if h, ok := volumes[collectionCell{row[0], plane, 2, x, y}]; ok && h > distance {
			return runeReject("RET_HOUSE_FURNITURE_GRID_OCCUPIED", "家具在不同平面体积重叠")
		}
		return nil
	}
	offset := func(v float64) int { return int(v / 0.35) }
	switch row[1] {
	case 0:
		for y := range ys {
			for h := 0; h < height; h++ {
				if e := check(1, y+offset(gz-az), h+offset(gy-ay), minx+offset(gx-ax), true); e != nil {
					return e
				}
			}
		}
		for x := range xs {
			for h := 0; h < height; h++ {
				if e := check(2, x+offset(gx-bx), h+offset(gy-by), gh-1-maxy+offset(bz-gz-float64(gh)*0.35), true); e != nil {
					return e
				}
			}
		}
	case 1:
		miny, maxx := mathGridMax(), -mathGridMax()
		for _, xy := range cells {
			if xy[1] < miny {
				miny = xy[1]
			}
			if xy[0] > maxx {
				maxx = xy[0]
			}
		}
		for x := range xs {
			for h := 0; h < height; h++ {
				if e := check(0, h+offset(ax-gx), x+offset(az-gz), miny+offset(ay-gy), false); e != nil {
					return e
				}
			}
		}
		for y := range ys {
			for h := 0; h < height; h++ {
				if e := check(2, h+offset(ax-bx), y+offset(ay-by), aw-1-maxx+offset(bz-az-float64(aw)*0.35), false); e != nil {
					return e
				}
			}
		}
	case 2:
		miny := mathGridMax()
		for _, xy := range cells {
			if xy[1] < miny {
				miny = xy[1]
			}
		}
		for x := range xs {
			for h := 0; h < height; h++ {
				if e := check(0, x+offset(bx-gx), gh-1-h+offset(bz-gz-float64(gh)*0.35), miny+offset(by-gy), false); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func mathGridMax() int { return 10000 }
func collectionLayoutRPC(p *Progress, method string, args []json.RawMessage) error {
	st := p.Collection
	rows := append([]CollectionPlacement(nil), st.Placements...)
	walls := st.Wallpapers
	integer := func(i int) (int, error) {
		var v int
		if i >= len(args) || json.Unmarshal(args[i], &v) != nil {
			return 0, errors.New("家具布置整数参数无效")
		}
		return v, nil
	}
	switch method {
	case "setup_furniture":
		if len(args) != 5 {
			return errors.New("摆放家具需要房间、平面、家具、朝向及坐标")
		}
		room, e := integer(0)
		if e != nil {
			return e
		}
		plane, e := integer(1)
		if e != nil {
			return e
		}
		id, e := integer(2)
		if e != nil {
			return e
		}
		orientation, e := integer(3)
		if e != nil {
			return e
		}
		var xy []int
		if json.Unmarshal(args[4], &xy) != nil || len(xy) != 2 {
			return errors.New("家具坐标无效")
		}
		layer := 2
		if containsInt([]int{3, 6}, collectionFurnitureKind(id)) {
			layer = 1
		}
		rows = append(rows, CollectionPlacement{room, plane, layer, xy[0], xy[1], id, orientation})
	case "withdraw_furniture":
		if len(args) != 4 {
			return errors.New("收起家具需要房间、平面、占用层及坐标")
		}
		room, e := integer(0)
		if e != nil {
			return e
		}
		plane, e := integer(1)
		if e != nil {
			return e
		}
		layer, e := integer(2)
		if e != nil {
			return e
		}
		var xy []int
		if json.Unmarshal(args[3], &xy) != nil || len(xy) != 2 {
			return errors.New("收起家具坐标无效")
		}
		found := false
		next := []CollectionPlacement{}
		for _, r := range rows {
			if r[0] == room && r[1] == plane && r[2] == layer && r[3] == xy[0] && r[4] == xy[1] {
				if collectionFurnitureKind(r[5]) == 99 {
					return runeReject("RET_HOUSE_FURNITURE_COUNT_ERROR", "设施家具不可收起")
				}
				found = true
			} else {
				next = append(next, r)
			}
		}
		if !found {
			return runeReject("RET_HOUSE_FURNITURE_COUNT_ERROR", "该位置没有家具")
		}
		rows = next
	case "update_restroom_grid_info", "update_restroom_grid_wallpapers":
		want := 3
		if method == "update_restroom_grid_wallpapers" {
			want = 4
		}
		if len(args) != want {
			return errors.New("家具布局保存参数无效")
		}
		var keys, values [][]int
		if json.Unmarshal(args[0], &keys) != nil || json.Unmarshal(args[1], &values) != nil || len(keys) != len(values) {
			return errors.New("家具布局键值列表无效")
		}
		rows = []CollectionPlacement{}
		for i, key := range keys {
			if len(key) != 5 || len(values[i]) != 2 {
				return errors.New("家具布局元组长度无效")
			}
			v := values[i]
			rows = append(rows, CollectionPlacement{key[0], key[1], key[2], key[3], key[4], v[0], v[1]})
		}
		var supplied map[string]json.RawMessage
		if json.Unmarshal(args[2], &supplied) != nil || supplied == nil {
			return errors.New("客户端家具库存结构无效")
		}
		if want == 4 {
			walls = map[int][]int{}
			if json.Unmarshal(args[3], &walls) != nil {
				return errors.New("收藏室墙纸映射无效")
			}
		}
	}
	if e := validateCollectionLayout(*p, rows, walls); e != nil {
		return e
	}
	old, new := map[[2]int]int{}, map[[2]int]int{}
	for _, r := range st.Placements {
		if collectionFurnitureKind(r[5]) == 99 {
			old[[2]int{r[0], r[5]}]++
		}
	}
	for _, r := range rows {
		if collectionFurnitureKind(r[5]) == 99 {
			new[[2]int{r[0], r[5]}]++
		}
	}
	for key, n := range old {
		if new[key] < n {
			return runeReject("RET_HOUSE_FURNITURE_COUNT_ERROR", "上传布局不得移除设施家具")
		}
	}
	st.Placements = rows
	st.Wallpapers = walls
	return nil
}
