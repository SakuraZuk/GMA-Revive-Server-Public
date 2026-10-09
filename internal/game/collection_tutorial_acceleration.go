package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

type CollectionTutorialReceipt struct {
	Facility int     `json:"facility"`
	Seconds  float64 `json:"seconds"`
	Time     int64   `json:"time"`
}

// 两份本版res完整解码的GuideFacilityAcc原件仅设施2/9000秒，任务6004或新版26004一次。
func (s *Service) collectionTutorialRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 || !remainingPolicyEnabled() {
		return nil, errors.New("收藏室教学加速需要已启用的玩家状态")
	}
	var facility int
	var seconds float64
	if json.Unmarshal(args[0], &facility) != nil || json.Unmarshal(args[1], &seconds) != nil || facility != 2 || seconds != 9000 {
		return nil, errors.New("教学加速与原生节点参数不匹配")
	}
	now := s.Now()
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if err := ensureCollection(p, now); err != nil {
			return err
		}
		taskID := 0
		for _, id := range []int{6004, 26004} {
			if p.GuideTasks[id].Status == 1 {
				if taskID != 0 {
					return errors.New("收藏室新旧教学状态冲突")
				}
				taskID = id
			}
		}
		if taskID == 0 {
			return errors.New("收藏室建材教学当前未进行")
		}
		st := p.Collection
		if st.TutorialAcceleration == nil {
			st.TutorialAcceleration = map[int]CollectionTutorialReceipt{}
		}
		if _, used := st.TutorialAcceleration[taskID]; used {
			return nil
		}
		f, owned := st.Facilities[facility]
		if !owned || f.Rate <= 0 {
			return errors.New("教学生产设施尚未拥有")
		}
		f.Keep = math.Min(f.Storage, f.Keep+seconds*f.Rate)
		if math.IsNaN(f.Keep) || math.IsInf(f.Keep, 0) {
			return errors.New("教学生产加速溢出")
		}
		st.Facilities[facility] = f
		st.TutorialAcceleration[taskID] = CollectionTutorialReceipt{facility, seconds, now.Unix()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return collectionPushes(c.SelectedAvatarUnsafe().Progress, now, false), nil
}
