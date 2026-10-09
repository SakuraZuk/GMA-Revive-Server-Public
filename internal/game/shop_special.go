package game

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ShopSubscription struct {
	Sequence  int64 `json:"sequence"`
	StartDay  int64 `json:"start_day"`
	Delivered int64 `json:"delivered"`
	Days      int64 `json:"days"`
	Bonus     int   `json:"bonus_id"`
}

// 开服限时商品必须使用本项目明确配置的服务开服时间，不能套用历史活动窗或角色创建时间。
func checkShopServerOpenDays(days int, now time.Time) error {
	if days <= 0 {
		return errors.New("商品开服天数配置无效")
	}
	raw := os.Getenv("HS_SERVER_OPEN_TIME")
	stamp, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || stamp <= 0 {
		return errors.New("开服限时商品需要明确HS_SERVER_OPEN_TIME Unix秒配置")
	}
	if now.Unix() < stamp {
		return runeReject("RET_SHOP_ACTIVITY_NOT_BEGIN", "服务尚未到明确开服时间")
	}
	if now.Unix()-stamp >= int64(days)*86400 {
		return runeReject("RET_SHOP_ACTIVITY_HAS_ENDED", "商品开服销售期限已结束")
	}
	return nil
}

type ShopGiftBoxRule struct {
	Bonus  int   `json:"bonus_id"`
	Count  int64 `json:"bonus_count"`
	Weight int64 `json:"bonus_weight"`
}
type ShopGiftBox struct {
	ID       int           `json:"box_id"`
	Received map[int]int64 `json:"bonus_receive_map"`
}

type shopRuneDrop struct {
	Positions []int64             `json:"pos_weight"`
	Stars     []int64             `json:"star_weight"`
	Suits     [][]json.RawMessage `json:"suit_id_weight"`
	Extra     [][]json.RawMessage `json:"extra_suit_id_weight"`
	Marks     []string            `json:"mark_base_attrs"`
}

// 契印商店实际购买子商品；权重、星级、套装及固定主属性全部来自Android掉落表。
func shopWeightedIndex(weights []int64) (int, error) {
	total := int64(0)
	for _, w := range weights {
		if w < 0 || total > math.MaxInt64-w {
			return 0, errors.New("契印掉落权重无效或溢出")
		}
		total += w
	}
	if total <= 0 {
		return 0, errors.New("契印掉落权重为空")
	}
	draw, e := rand.Int(rand.Reader, big.NewInt(total))
	if e != nil {
		return 0, e
	}
	n := draw.Int64()
	for i, w := range weights {
		if n < w {
			return i, nil
		}
		n -= w
	}
	return 0, errors.New("契印权重抽取失败")
}
func shopWeightedSuit(rows [][]json.RawMessage, optional bool) (int, error) {
	ids := []int{}
	weights := []int64{}
	for _, row := range rows {
		if len(row) != 2 {
			return 0, errors.New("契印套装权重结构无效")
		}
		var id int
		var weight int64
		if json.Unmarshal(row[0], &id) != nil || json.Unmarshal(row[1], &weight) != nil || weight < 0 || id < 0 || (weight > 0 && id == 0) {
			return 0, errors.New("契印套装权重没有明确模板")
		}
		ids = append(ids, id)
		weights = append(weights, weight)
	}
	if optional {
		zero := true
		for _, w := range weights {
			zero = zero && w == 0
		}
		if zero {
			return 0, nil
		}
	}
	i, e := shopWeightedIndex(weights)
	if e != nil {
		return 0, e
	}
	return ids[i], nil
}
func grantShopRandomRunes(p *Progress, rows []json.RawMessage, amount int64, now time.Time) error {
	if amount <= 0 || amount > 10000 {
		return errors.New("随机契印奖励批量无效")
	}
	for _, raw := range rows {
		var fields []json.RawMessage
		var dropID int
		var counts []int64
		var chance float64
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields[0], &dropID) != nil || json.Unmarshal(fields[1], &counts) != nil || json.Unmarshal(fields[2], &chance) != nil {
			return errors.New("随机契印奖励结构无效")
		}
		if len(counts) == 0 || chance < 0 || chance > 1 || math.IsNaN(chance) || math.IsInf(chance, 0) {
			return errors.New("随机契印数量或概率无效")
		}
		minimum, maximum := counts[0], counts[0]
		for _, n := range counts {
			if n < 0 || n > 10000 {
				return errors.New("随机契印数量无效")
			}
			if n < minimum {
				minimum = n
			}
			if n > maximum {
				maximum = n
			}
		}
		if maximum > 10000/amount {
			return errors.New("随机契印奖励过大")
		}
		drop, known := androidShop.RuneDrops[dropID]
		if !known || len(drop.Positions) != 4 || len(drop.Stars) != 5 {
			return errors.New("随机契印掉落表缺失")
		}
		// 原生D44DBBA7：每份先probability_choice，再randint(min(count_list),max(count_list))。
		for batch := int64(0); batch < amount; batch++ {
			chosen, e := shopProbabilityChoice(chance)
			if e != nil {
				return e
			}
			if !chosen {
				continue
			}
			draw, e := rand.Int(rand.Reader, big.NewInt(maximum-minimum+1))
			if e != nil {
				return e
			}
			for i := int64(0); i < minimum+draw.Int64(); i++ {
				pos, e := shopWeightedIndex(drop.Positions)
				if e != nil {
					return e
				}
				star, e := shopWeightedIndex(drop.Stars)
				if e != nil {
					return e
				}
				suitRows, e := nativeRuneSuitRows(dropID, drop.Suits)
				if e != nil {
					return e
				}
				suit, e := shopWeightedSuit(suitRows, false)
				if e != nil {
					return e
				}
				extra, e := shopWeightedSuit(drop.Extra, true)
				if e != nil {
					return e
				}
				if _, e := GrantRuneWithMarks(p, RuneSpec{Suit: suit, Position: pos + 1, Star: star + 1, Level: 1, ExtraSuit: extra}, drop.Marks, now); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

func shopProbabilityChoice(chance float64) (bool, error) {
	if chance < 0 || chance > 1 || math.IsNaN(chance) || math.IsInf(chance, 0) {
		return false, errors.New("奖励概率无效")
	}
	if chance == 0 {
		return false, nil
	}
	if chance == 1 {
		return true, nil
	}
	// 以浮点值的精确有理数表示抽样，避免固定百分比精度截断。
	ratio := new(big.Rat).SetFloat64(chance)
	draw, e := rand.Int(rand.Reader, ratio.Denom())
	if e != nil {
		return false, e
	}
	return draw.Cmp(ratio.Num()) < 0, nil
}

func nextShopDay(now time.Time) int64 {
	z := time.FixedZone("北京时间", 8*3600)
	t := now.In(z)
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, z).Unix()
}
func resolveShopBonus(id int, stamp int64) int {
	b, known := androidShop.Bonuses[id]
	if !known {
		return 0
	}
	if b.ActivityBonus > 0 && (len(b.Begin) > 0 || len(b.End) > 0) && shopTimeWindow(b.Begin, b.End, stamp) == "" {
		return b.ActivityBonus
	}
	return id
}
func grantShopBonus(p *Progress, id int, amount int64, now time.Time, changes map[int]int64, cards *[]string, depth int) error {
	return grantNativeBonusAssets(p, id, amount, 0, now, changes, cards, depth)
}
func grantNativeBonusAssets(p *Progress, id int, amount int64, level int, now time.Time, changes map[int]int64, cards *[]string, depth int) error {
	if depth > 8 || amount <= 0 || amount > 10000 {
		return errors.New("商店奖励嵌套或批量过大")
	}
	b, known := androidShop.Bonuses[id]
	if !known || b.Lucky != 0 || (len(b.Inner) > 0 && string(b.Inner) != "null") {
		return errors.New("商店奖励包含未取证动态规则")
	}
	if alternate := resolveShopBonus(id, now.Unix()); alternate != id {
		return grantNativeBonusAssets(p, alternate, amount, level, now, changes, cards, depth+1)
	}
	if len(b.Fixed)+len(b.RandomRunes)+len(b.RandomItems)+len(b.RandomLibs) == 0 {
		return errors.New("商店奖励为空")
	}
	for _, row := range b.Fixed {
		if len(row) != 2 || row[1] <= 0 || row[1] > math.MaxInt64/amount {
			return errors.New("商店奖励数量无效")
		}
		if e := grantNativeItem(p, int(row[0]), row[1]*amount, level, now, changes, cards, depth+1); e != nil {
			return e
		}
	}
	if err := grantShopRandomRunes(p, b.RandomRunes, amount, now); err != nil {
		return err
	}
	if err := grantNativeRandomItems(p, b.RandomItems, amount, level, now, changes, cards, depth+1); err != nil {
		return err
	}
	return grantNativeItemLibraries(p, b.RandomLibs, amount, level, now, changes, cards, depth+1)
}

// 玩家事务内发月卡邮件；每日发行进度随邮件同存，删除已发邮件不会重发。
// 原生SHOP_MAIL_VALID_DURATION=30天，UTC+8零点；过期日奖励推进但不生成已过期邮件。
func refreshShopSubscription(s *Service, p *Progress, now time.Time) error {
	ids := []int{}
	for id, d := range p.CommodityDetails {
		if len(d.SubscribeJobs) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		d := p.CommodityDetails[id]
		if d.LastPurchase > now.Unix() {
			return errors.New("订阅存档时间回拨")
		}
		keep := []ShopSubscription{}
		for _, job := range d.SubscribeJobs {
			if job.Sequence <= 0 || job.StartDay <= 0 || job.Days != 30 || job.Delivered < 1 || job.Delivered > job.Days || job.Bonus <= 0 {
				return errors.New("订阅发行任务存档无效")
			}
			for job.Delivered < job.Days {
				due := job.StartDay + (job.Delivered-1)*86400
				if due > now.Unix() {
					break
				}
				expires := due + 30*86400
				if expires > now.Unix() {
					bid := resolveShopBonus(job.Bonus, due)
					b, known := androidShop.Bonuses[bid]
					if !known || len(b.Fixed) == 0 || len(b.RandomItems)+len(b.RandomRunes)+len(b.RandomLibs) > 0 || (len(b.Inner) > 0 && string(b.Inner) != "null") {
						return errors.New("订阅日奖励目录无效")
					}
					attachments := map[int]int64{}
					for _, row := range b.Fixed {
						if len(row) != 2 || row[0] <= 0 || row[1] <= 0 || attachments[int(row[0])] > math.MaxInt64-row[1] {
							return errors.New("订阅日奖励数量无效")
						}
						attachments[int(row[0])] += row[1]
					}
					digest := sha256.Sum256([]byte(fmt.Sprintf("幻书月卡:%d:%d:%d", id, job.Sequence, job.Delivered)))
					mid := int(binary.BigEndian.Uint32(digest[:4]) & 0x7fffffff)
					if mid == 0 {
						return errors.New("月卡邮件编号无效")
					}
					uuid := hex.EncodeToString(digest[4:16])
					template := androidShop.MailTemplates[job.Bonus]
					title := template.Title
					if title == "" {
						return errors.New("月卡邮件模板缺失")
					}
					content := strings.ReplaceAll(template.Content, "%s", fmt.Sprintf("%d天", job.Days-job.Delivered))
					if e := s.prepareAndInsertMail(p, Mail{MID: mid, UUID: uuid, Title: title, Content: content, Sender: "阿克夏书馆", Attachments: attachments, CreatedAt: due, ExpiresAt: expires}); e != nil {
						return e
					}
				}
				if d.SubscribeCount == math.MaxInt64 {
					return errors.New("订阅发行天数溢出")
				}
				d.SubscribeCount++
				job.Delivered++
			}
			if job.Delivered < job.Days {
				keep = append(keep, job)
			}
		}
		d.SubscribeJobs = keep
		p.CommodityDetails[id] = d
	}
	return nil
}

func grantShopGiftBox(p *Progress, id int, now time.Time, changes map[int]int64, cards *[]string) error {
	rules, known := androidShop.GiftBoxes[id]
	if !known || len(rules) == 0 {
		return errors.New("随机礼盒目录缺失")
	}
	if p.GiftBoxInfo == nil {
		p.GiftBoxInfo = map[int]ShopGiftBox{}
	}
	box := p.GiftBoxInfo[id]
	if box.ID == 0 {
		box = ShopGiftBox{ID: id, Received: map[int]int64{}}
	}
	if box.Received == nil {
		box.Received = map[int]int64{}
	}
	total := int64(0)
	weights := []int64{}
	for _, r := range rules {
		held := box.Received[r.Bonus]
		if r.Weight <= 0 || r.Count <= 0 || held < 0 || held > r.Count || r.Count-held > math.MaxInt64/r.Weight {
			return errors.New("随机礼盒权重或已领奖次数无效")
		}
		w := r.Weight * (r.Count - held)
		if total > math.MaxInt64-w {
			return errors.New("随机礼盒权重溢出")
		}
		total += w
		weights = append(weights, w)
	}
	if total <= 0 {
		return errors.New("随机礼盒未正确重置")
	}
	draw, e := rand.Int(rand.Reader, big.NewInt(total))
	if e != nil {
		return e
	}
	value := draw.Int64()
	selected := 0
	for i, w := range weights {
		if value < w {
			selected = rules[i].Bonus
			break
		}
		value -= w
	}
	if selected <= 0 {
		return errors.New("随机礼盒抽取失败")
	}
	if e := grantShopBonus(p, selected, 1, now, changes, cards, 0); e != nil {
		return e
	}
	box.Received[selected]++
	// 原生gift_box_detail.generate_bonus在最后一个奖励抽完后自动reset_box。
	finished := true
	for _, r := range rules {
		if box.Received[r.Bonus] < r.Count {
			finished = false
		}
	}
	if finished {
		box.Received = map[int]int64{}
	}
	p.GiftBoxInfo[id] = box
	return nil
}

func (s *Service) resetShopGiftBoxRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("重置随机礼盒需要礼盒编号")
	}
	var id int
	if json.Unmarshal(args[0], &id) != nil || id <= 0 {
		return nil, errors.New("随机礼盒编号无效")
	}
	if _, known := androidShop.GiftBoxes[id]; !known {
		return nil, errors.New("随机礼盒不存在")
	}
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.GiftBoxInfo == nil {
			p.GiftBoxInfo = map[int]ShopGiftBox{}
		}
		p.GiftBoxInfo[id] = ShopGiftBox{ID: id, Received: map[int]int64{}}
		return nil
	}); e != nil {
		return nil, e
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"gift_box_info", c.SelectedAvatarUnsafe().Progress.GiftBoxInfo}), push("Avatar", "on_reset_gift_box")}, nil
}
