package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestShopRecommendationExplicitOfferAndServerOpenGate(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{Materials: map[int]Material{10: {ID: 10, Count: 1000, Total: 1000}}, ClearedDungeons: []int{504}}
	p.UnlockSystems = map[string]int{androidShop.Shops[801].System: 1}
	if _, e := buyCommodity(&p, 8010001, 1, 0, 60, now); e == nil {
		t.Fatal("没有服务端offer仍出售推荐商品")
	}
	offer := ShopRecommendation{ID: 801001, Commodity: 8010001, Price: 648, ShowDuration: 120, Expired: now.Add(time.Hour).Unix()}
	if e := issueShopRecommendation(&p, offer, now); e != nil {
		t.Fatal(e)
	}
	wire := recommendationProperties(p)[801001].(map[string]any)
	var gifts []map[string]any
	if json.Unmarshal([]byte(wire["gifts"].(string)), &gifts) != nil || len(gifts) != 1 || gifts[0]["price"] != float64(648) || wire["state"] != nil {
		t.Fatal("推荐原生JSON字符串字段错误", wire)
	}
	box, e := buyCommodity(&p, 8010001, 1, 0, 60, now)
	if e != nil || p.Materials[10].Count != 352 || box["materials"].(map[int]int64)[501] != 2 || p.CommodityDetails[8010001].Recommendation.State != 3 {
		t.Fatal("推荐价格或原生奖励没有真实结算", e)
	}
	if _, e = buyCommodity(&p, 8010001, 1, 0, 60, now); e == nil {
		t.Fatal("推荐限购允许重复")
	}
	q := Progress{}
	offer.Price = 1
	if e := issueShopRecommendation(&q, offer, now); e == nil {
		t.Fatal("管理offer能跳过折扣区间")
	}
	t.Setenv("HS_SERVER_OPEN_TIME", "")
	if checkShopServerOpenDays(7, now) == nil {
		t.Fatal("缺开服时间仍放行开服限时商品")
	}
	t.Setenv("HS_SERVER_OPEN_TIME", "1800000000")
	if e := checkShopServerOpenDays(7, now); e != nil {
		t.Fatal(e)
	}
	if checkShopServerOpenDays(7, now.Add(7*24*time.Hour)) == nil {
		t.Fatal("开服限时到期仍放行")
	}
}

func TestShopExplicitRuneFallbackPolicyAndSelfSelectedBonus(t *testing.T) {
	now := time.Unix(1800000000, 0)
	p := Progress{}
	t.Setenv("HS_RUNE_FALLBACK_POOLS", "")
	rows := []json.RawMessage{json.RawMessage(`[400,[2],1]`)}
	if grantShopRandomRunes(&p, rows, 1, now) == nil {
		t.Fatal("Android全零套装权重被猜测发奖")
	}
	t.Setenv("HS_RUNE_FALLBACK_POOLS", `{"400":[[1101,100]]}`)
	if e := grantShopRandomRunes(&p, rows, 1, now); e != nil || len(p.Runes) != 2 {
		t.Fatal("明确本服套装候选池未生效", e)
	}
	for _, r := range p.Runes {
		if r.Suit != 1101 {
			t.Fatal("本服候选池以外套装被发放")
		}
	}
	changes := map[int]int64{}
	cards := []string{}
	if e := grantNativeSelectedBonus(&p, 200933, 3, 110, 60, now, changes, &cards, 0); e != nil || p.Materials[110].Count != 3 || p.Materials[120].Count != 0 {
		t.Fatal("自选奖励未按实际选择过滤", e)
	}
	if e := grantNativeSelectedBonus(&p, 200933, 1, 11, 60, now, changes, &cards, 0); e == nil {
		t.Fatal("自选接口接受目录外材料")
	}
}
