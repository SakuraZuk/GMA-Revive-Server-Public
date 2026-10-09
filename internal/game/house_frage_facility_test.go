package game

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

// 固定原生残页概率的边界随机值，之后的奖励权重选择取第一项。
// 不用大量随机样本掩盖小额入住加成是否接到真实领奖事务。
type houseFrageBoundaryEntropy struct {
	bytes []byte
}

func (r *houseFrageBoundaryEntropy) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
		if len(r.bytes) > 0 {
			p[i] = r.bytes[0]
			r.bytes = r.bytes[1:]
		}
	}
	return len(p), nil
}
func houseFrageWithBoundary(sample float64, fn func()) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(sample*float64(1<<53)))
	previous := rand.Reader
	rand.Reader = &houseFrageBoundaryEntropy{bytes: append([]byte{}, b[1:]...)}
	defer func() { rand.Reader = previous }()
	fn()
}
func houseFrageOccupiedFixture(now time.Time) Progress {
	p := NewProgress(20, now)
	_ = ensureCollection(&p, now)
	p.Collection.Facilities[6] = CollectionFacility{ID: 6, Level: 1}
	room := newCollectionRoom(1, now)
	room.Cards, room.Slots = map[int]int{4401: 1}, map[int]int{1: 4401}
	p.Collection.Rooms[1] = room
	ensureHouseFrage(&p)
	return p
}

func TestActivityHouseFacilitySSRNativeScopeAndAdditiveUp(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	p := houseFrageOccupiedFixture(now)
	bonus, err := houseFrageFacilitySSRBonus(&p)
	if err != nil || math.Abs(bonus-0.01) > 1e-12 {
		t.Fatal("设施6未取原生实际房间一名入住加成", bonus, err)
	}
	// 另一房间的入住不叠加到设施6，也不将入住加成乘生产设施的4倍。
	other := newCollectionRoom(2, now)
	other.Slots = map[int]int{1: 4409, 2: 4402}
	p.Collection.Rooms[2] = other
	if bonus, err = houseFrageFacilitySSRBonus(&p); err != nil || math.Abs(bonus-0.01) > 1e-12 {
		t.Fatal("全收藏室人数混入骰子桌实际房间加成", bonus, err)
	}
	w := p.Activities.House
	w.Up = 4
	// 原生SSR基础概率40%，UP40%与入住1%相加后为56.4%。
	// 56.5%必须落代币；连续相乘会错误得到56.56%并发残页。
	houseFrageWithBoundary(0.565, func() {
		mid, count, card, e := houseFrageCardBonus(&p, &HouseFrageSite{ID: 2, Card: 2404}, 1)
		if e != nil || card || mid != 20 || count != 20 || w.Up != 3 {
			t.Fatal("原生UP与入住加成没有按加法合并", mid, count, card, e, w.Up)
		}
	})
	w.Up = 0
	houseFrageWithBoundary(0.804, func() {
		mid, _, card, e := houseFrageCardBonus(&p, &HouseFrageSite{ID: 2, Card: 3201}, 1)
		if e != nil || card || mid != 18 {
			t.Fatal("典藏专属入住加成错误作用精装残页", mid, card, e)
		}
	})
	w.Up = 4
	houseFrageWithBoundary(0.999, func() {
		mid, count, card, e := houseFrageCardBonus(&p, &HouseFrageSite{ID: 2, Card: 2404, Fixed: 2404}, 1)
		if e != nil || !card || mid != houseFrageFragment(2404) || count != 1 || w.Up != 4 {
			t.Fatal("固定祈愿错误使用随机概率或消耗UP", mid, count, card, e, w.Up)
		}
	})
	room := p.Collection.Rooms[1]
	room.Slots = map[int]int{1: 4401, 2: 4402, 3: 4403, 4: 4404, 5: 4405, 6: 4406}
	p.Collection.Rooms[1] = room
	if bonus, err = houseFrageFacilitySSRBonus(&p); err != nil || bonus != 0 {
		t.Fatal("入住超原生表长度时擅自按末档封顶", bonus, err)
	}
}

func TestActivityHouseFacilitySSRActualMovePersistenceAndRollback(t *testing.T) {
	ctx := context.Background()
	a := NewFixtureAccounts(nil)
	s := New(a, nil)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	c, _ := newBattleConnection(t, ctx, a, s)
	fragment := houseFrageFragment(2404)
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		occupied := houseFrageOccupiedFixture(now)
		p.Collection = occupied.Collection
		p.Activities.House = occupied.Activities.House
		p.Activities.House.Sites = map[int]*HouseFrageSite{1: {ID: 1}, 2: {ID: 2, Card: 2404}}
		p.Activities.House.Pos = 1
		p.Materials[402] = Material{ID: 402, Count: 1, Total: 1}
		p.Materials[fragment] = Material{ID: fragment}
		p.Materials[20] = Material{ID: 20}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	move := func() {
		t.Helper()
		if _, err := s.Handle(ctx, c, "house_frage_ctrl_move", []json.RawMessage{json.RawMessage("1"), json.RawMessage("1")}); err != nil {
			t.Fatal(err)
		}
	}
	// 40.2%位于40%基础与40.4%真实入住概率之间。
	houseFrageWithBoundary(0.402, move)
	p := c.SelectedAvatarUnsafe().Progress
	if p.Materials[402].Count != 0 || p.Materials[fragment].Count != 1 || p.Materials[20].Count != 0 || p.Activities.House.SSR != 1 {
		t.Fatal("真实扣骰子与入住典藏残页判定未同事务入账", p.Materials[fragment], p.Materials[20], p.Activities.House)
	}
	stored, err := a.QuickLogin(ctx, ClientInfo{Account: "战斗测试", Password: "pw", Hostnum: 1})
	if err != nil || stored.Avatars[0].Progress.Materials[fragment].Count != 1 {
		t.Fatal("设施入住奖励未持久化", err)
	}
	raw, _ := json.Marshal(stored.Avatars[0].Progress)
	var restored Progress
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if bonus, e := houseFrageFacilitySSRBonus(&restored); e != nil || math.Abs(bonus-0.01) > 1e-12 {
		t.Fatal("存档重载后丢失实际入住加成", bonus, e)
	}
	before, _ := json.Marshal(p)
	houseFrageWithBoundary(0.402, move)
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("不足骰子失败错误增加入住奖励或推进棋盘")
	}
	if err = s.updateProgress(ctx, c, func(p *Progress) error {
		p.Activities.House.Pos = 1
		p.Materials[402] = Material{ID: 402, Count: 1, Total: 2}
		room := p.Collection.Rooms[1]
		room.Cards, room.Slots = map[int]int{}, map[int]int{}
		p.Collection.Rooms[1] = room
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	houseFrageWithBoundary(0.402, move)
	p = c.SelectedAvatarUnsafe().Progress
	if p.Materials[fragment].Count != 1 || p.Materials[20].Count != 20 || p.Activities.House.SSR != 1 {
		t.Fatal("空入住仍使用缓存典藏加成", p.Materials[fragment], p.Materials[20], p.Activities.House)
	}
}
