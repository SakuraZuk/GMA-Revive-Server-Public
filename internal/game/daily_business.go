package game

import "context"

// 登录先补离线日界；在线跨日一次事务推进订阅和特殊送礼，失败不消耗发行状态。
func (s *Service) refreshDailyState(ctx context.Context, c *Connection) ([]Push, error) {
	if s.Now().Unix() < c.nextDailyRetry {
		return nil, nil
	}
	if err := s.refreshActivityAwards(ctx, c); err != nil {
		c.nextDailyRetry = s.Now().Unix() + 60
		return nil, err
	}
	day := socialDay(s.Now())
	if c.lastDailyCheckDay == "" {
		c.lastDailyCheckDay = day
		return nil, nil
	}
	if day <= c.lastDailyCheckDay || s.Now().Unix() < c.nextDailyRetry {
		return nil, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureBasicRewards(p, s.Now(), true); err != nil {
			return err
		}
		if err := refreshShopSubscription(s, p, s.Now()); err != nil {
			return err
		}
		if err := ensureActivityLogin(p, p.AvatarLevel, s.Now()); err != nil {
			return err
		}
		if p.SpecialGiftDay != "" && day > p.SpecialGiftDay {
			p.SpecialGiftCounts = map[int]int{}
			p.SpecialGiftDay = day
		}
		return nil
	})
	if err != nil {
		c.nextDailyRetry = s.Now().Unix() + 60
		return nil, err
	}
	result := []Push{push("Avatar", "client_prop_changed", []any{"special_gift_card_2_count", specialGiftCounts(c.SelectedAvatarUnsafe().Progress, s.Now())}), push("Avatar", "client_prop_changed", []any{"commodity_detail_info", commodityProperties(c.SelectedAvatarUnsafe().Progress, s.Now())})}
	extra, err := s.refreshSocialDaily(ctx, c)
	if err != nil {
		c.nextDailyRetry = s.Now().Unix() + 60
		return result, err
	}
	c.lastDailyCheckDay = day
	c.nextDailyRetry = 0
	return append(result, extra...), nil
}
