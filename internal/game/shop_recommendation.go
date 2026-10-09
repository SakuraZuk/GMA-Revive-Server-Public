package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"
)

type ShopRecommendation struct {
	ID           int   `json:"recommend_id"`
	Commodity    int   `json:"commodity_id"`
	Price        int64 `json:"price"`
	ShowDuration int64 `json:"show_duration"`
	Expired      int64 `json:"expired_time"`
	ShowTime     int64 `json:"show_time"`
	Issued       int64 `json:"issued_time"`
	State        int   `json:"state"`
}

func recommendationProperties(p Progress) map[int]any {
	out := map[int]any{}
	for _, detail := range p.CommodityDetails {
		if r := detail.Recommendation; r != nil {
			gifts, _ := json.Marshal([]map[string]any{{"id": r.Commodity, "price": r.Price, "show_duration": r.ShowDuration, "expired_time": r.Expired}})
			out[r.ID] = map[string]any{"gifts": string(gifts), "show_time": r.ShowTime}
		}
	}
	return out
}

func validShopRecommendation(d CommodityDetail, now time.Time) error {
	r := d.Recommendation
	if r == nil || r.ID <= 0 || r.Commodity != d.ID || r.Price <= 0 || r.ShowDuration <= 0 || r.Issued <= 0 || r.Issued > now.Unix() || r.ShowTime > now.Unix() {
		return runeReject("RET_SHOP_INVALID", "推荐商品没有有效服务端报价")
	}
	if r.State == 3 {
		return runeReject("RET_SHOP_GOOD_HAS_SOLD_OUT", "推荐礼包已经购买")
	}
	if r.ShowTime == 0 {
		if r.Expired <= now.Unix() {
			return runeReject("RET_SHOP_INVALID", "推荐报价未展示即过期")
		}
	} else if now.Unix()-r.ShowTime >= r.ShowDuration*60 {
		return runeReject("RET_SHOP_INVALID", "推荐报价展示期限已结束")
	}
	return nil
}

// 发行端明确提供报价与展示期限；Android没有外部推荐算法，不自动猜折扣/发行策略。
// 必须由管理接口或明确运营配置在玩家事务中调用。
func issueShopRecommendation(p *Progress, r ShopRecommendation, now time.Time) error {
	rule, known := androidShop.Commodities[r.Commodity]
	if !known || rule.Type != 3 || r.ID <= 0 || r.Price <= 0 || len(rule.Prices) != 1 || len(rule.DiscountRange) != 2 || r.ShowDuration <= 0 || r.ShowDuration > 24*60 || r.Expired <= now.Unix() || r.Expired-now.Unix() > 30*86400 {
		return errors.New("推荐报价、期限或商品类型无效")
	}
	minimum, maximum := rule.DiscountRange[0], rule.DiscountRange[1]
	if minimum <= 0 || maximum < minimum || maximum > 1 || math.IsNaN(minimum+maximum) || math.IsInf(minimum+maximum, 0) || r.Price < int64(float64(rule.Prices[0])*minimum) || r.Price > int64(float64(rule.Prices[0])*maximum) {
		return errors.New("推荐报价不在Android折扣区间")
	}
	for id, d := range p.CommodityDetails {
		if id != r.Commodity && d.Recommendation != nil && d.Recommendation.ID == r.ID {
			return errors.New("推荐编号已由其他商品使用")
		}
	}
	d := p.CommodityDetails[r.Commodity]
	if d.ID == 0 {
		d = CommodityDetail{ID: r.Commodity}
	}
	if d.Total > 0 && rule.Total > 0 && d.Total >= rule.Total {
		return runeReject("RET_SHOP_GOOD_HAS_SOLD_OUT", "推荐礼包已达终身限购")
	}
	if d.RecommendTime > 0 && (now.Unix() < d.RecommendTime || now.Unix()-d.RecommendTime < rule.RecommendInterval*3600) {
		return errors.New("推荐商品尚在发行间隔内")
	}
	r.Issued = now.Unix()
	r.ShowTime = 0
	r.State = 0
	d.Time = now.Unix()
	d.RecommendTime = now.Unix()
	d.Recommendation = &r
	if p.CommodityDetails == nil {
		p.CommodityDetails = map[int]CommodityDetail{}
	}
	p.CommodityDetails[r.Commodity] = d
	return nil
}

func (s *Service) updateRecommendationRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("推荐状态需要商品编号与状态")
	}
	var id, state int
	if json.Unmarshal(args[0], &id) != nil || json.Unmarshal(args[1], &state) != nil || id <= 0 || (state != 1 && state != 2) {
		return nil, errors.New("推荐状态参数无效")
	}
	if e := s.updateProgress(ctx, c, func(p *Progress) error {
		d := p.CommodityDetails[id]
		if e := validShopRecommendation(d, s.Now()); e != nil {
			return e
		}
		if state > d.Recommendation.State {
			if d.Recommendation.ShowTime == 0 {
				d.Recommendation.ShowTime = s.Now().Unix()
			}
			d.Recommendation.State = state
			p.CommodityDetails[id] = d
		}
		return nil
	}); e != nil {
		return nil, e
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"recommend_gifts", recommendationProperties(c.SelectedAvatarUnsafe().Progress)})}, nil
}
