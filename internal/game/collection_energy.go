package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

// 原生F0E40550只接收房间编号，时间由服务器确定；下行Int也是房间编号。
// 能量的显示值由Android calculate_energy_info/init_energy根据真实入住、设施及性格计算。
// 读取包含空房：原生get在未启动时也记录now，随后原生回调重算能量显示参数。
func (s *Service) collectionEnergyRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("收藏室能量同步需要房间编号")
	}
	var roomID int
	if json.Unmarshal(args[0], &roomID) != nil || roomID <= 0 {
		return nil, errors.New("收藏室能量房间编号无效")
	}
	e := s.updateProgress(ctx, c, func(p *Progress) error {
		if e := ensureCollection(p, s.Now()); e != nil {
			return e
		}
		if remainingPolicyEnabled() {
			r, err := collectionRoom(p, roomID)
			if err != nil {
				return err
			}
			r.Energy["last_get_time"] = float64(s.Now().UnixNano()) / 1e9
			p.Collection.Rooms[roomID] = r
			return nil
		}
		r, e := collectionRoom(p, roomID)
		if e != nil {
			return e
		}
		stamp := float64(s.Now().UnixNano()) / 1e9
		if r.Energy == nil {
			r.Energy = newCollectionRoom(roomID, s.Now()).Energy
		}
		if value, ok := r.Energy["last_get_time"]; ok {
			var last float64
			switch typed := value.(type) {
			case float64:
				last = typed
			case int64:
				last = float64(typed)
			case int:
				last = float64(typed)
			default:
				return errors.New("收藏室能量时间存档类型无效")
			}
			if math.IsNaN(last) || math.IsInf(last, 0) || last < 0 || last > stamp {
				return errors.New("收藏室能量时间无效或时钟回拨")
			}
		}
		r.Energy["last_get_time"] = stamp
		p.Collection.Rooms[roomID] = r
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := collectionPushes(c.SelectedAvatarUnsafe().Progress, s.Now(), false)
	return append(out, push("Avatar", "on_last_get_time_update", roomID)), nil
}
