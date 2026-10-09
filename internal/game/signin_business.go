package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

func (s *Service) signinMonthly(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("月签到需要空参数")
	}
	now := s.Now().In(time.FixedZone("Asia/Shanghai", 8*3600))
	month, day := now.Format("2006-01"), now.Format("2006-01-02")
	granted := map[string]any{}
	var times int
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if p.SigninMonth != month {
			p.SigninMonth, p.SigninTimes, p.SigninDay = month, 0, ""
		}
		monthDays := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
		if p.SigninTimes < 0 || p.SigninTimes > monthDays {
			return errors.New("月签到存档次数无效")
		}
		if p.SigninDay != day && p.SigninTimes < monthDays {
			reward, exists := androidMonthlySignin[p.SigninTimes+1]
			if !exists {
				return errors.New("Android 月签到奖励缺失")
			}
			if p.Materials == nil {
				p.Materials = map[int]Material{}
			}
			for id, amount := range reward {
				m := p.Materials[id]
				if id <= 0 || amount <= 0 || m.Count > math.MaxInt64-int64(amount) || m.Total > math.MaxInt64-int64(amount) {
					return errors.New("月签到奖励或材料容量无效")
				}
				m.ID, m.Count, m.Total = id, m.Count+int64(amount), m.Total+int64(amount)
				p.Materials[id] = m
				granted[strconv.Itoa(id)] = int64(amount)
			}
			p.SigninTimes++
			p.SigninDay = day
		}
		times = p.SigninTimes
		return nil
	}); err != nil {
		return nil, err
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"signin_info", map[string]any{"__custom_type": "signin_info.signin_info", "monthly_times": times, "monthly_today_flag": true}}), push("Avatar", "client_prop_changed", []any{"material_mgr", materialProperties(c.identity.Avatars, c.hostnum)}), push("Avatar", "on_signin_monthly", 0, map[string]any{"__custom_type": "box.box", "materials": granted})}, nil
}

func signinProperties(p Progress, now time.Time) map[string]any {
	now = now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	times := p.SigninTimes
	if p.SigninMonth != now.Format("2006-01") {
		times = 0
	}
	return map[string]any{"__custom_type": "signin_info.signin_info", "monthly_times": times, "monthly_today_flag": p.SigninMonth == now.Format("2006-01") && p.SigninDay == now.Format("2006-01-02")}
}
