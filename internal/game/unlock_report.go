package game

import (
	"context"
	"encoding/json"
	"errors"
)

// 原生on_unlock_system为无回调的系统名称列表；仍由服务端原表条件授权，不能信任客户端名称直接解锁。
func (s *Service) reportUnlockedSystems(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("系统解锁上报需要登录角色及名称列表")
	}
	var names []string
	if json.Unmarshal(args[0], &names) != nil || len(names) > len(clientBaseline.SystemUnlocks) {
		return nil, errors.New("系统解锁名称列表无效")
	}
	changed := false
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		for _, name := range names {
			known := false
			for _, def := range clientBaseline.SystemUnlocks {
				if def.System != name {
					continue
				}
				known = true
				raw, err := json.Marshal(def.Conditions)
				if err != nil {
					return err
				}
				if err := (activityRow{"unlock_condition": raw}).conditions("unlock_condition", *p, p.AvatarLevel); err != nil {
					return errors.New("系统解锁上报尚未满足原表条件")
				}
				if p.UnlockSystems == nil {
					p.UnlockSystems = map[string]int{}
				}
				if p.UnlockSystems[name] < def.Version {
					p.UnlockSystems[name] = def.Version
					changed = true
				}
				break
			}
			if !known {
				return errors.New("系统解锁名称不存在")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"unlock_systems", c.SelectedAvatarUnsafe().Progress.UnlockSystems})}, nil
}
