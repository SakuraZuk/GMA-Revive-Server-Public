package game

import (
	"context"
	"encoding/json"
	"errors"
)

// 原生server_controler.exit_battle没有参数。主动退出与TCP断线严格分开，
// 双方存档、积分、收据和投递由同一双角色事务提交。
func (s *Service) humanExitBattle(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("真人退出战斗需要已登录角色且不接受参数")
	}
	local := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if local == nil {
		return nil, errors.New("没有可退出的真人房间")
	}
	room, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		for _, participant := range r.IDs {
			b := v[participant].Progress.Battle
			if b == nil || b.UUID != r.UUID {
				return errors.New("真人退出战斗的双方会话归属失配")
			}
		}
		if r.Status == "settled" || r.Status == "aborted" {
			return nil // 已提交胜负不能被后续退出改写，也不再产生投递。
		}
		if len(r.Results) != 0 || r.Native != nil && len(r.Native.Winners) != 0 {
			// Android路线仍要求两端原生结果一致；不能以退出覆盖已收到的末态。
			return errors.New("真人已有原生末态，等待另一端确认，退出不能改写胜方")
		}
		if r.Status != "fighting" {
			humanAbort(r, v, "开战前主动退出")
			return nil
		}
		if len(r.IDs) != 2 {
			return errors.New("真人弃权房间必须恰好包含两个角色")
		}
		for _, participant := range r.IDs {
			player := r.Players[participant]
			if !player.Started || !player.Ready {
				humanAbort(r, v, "共同开战屏障完成前主动退出")
				return nil
			}
			if participant != id {
				r.Winner = participant
			}
		}
		r.Reason = "player_forfeit"
		return settleHumanPvp(r, v, s.Now().Unix())
	})
	if err != nil {
		return nil, err
	}
	if room.Native != nil && (room.Status == "settled" || room.Status == "aborted") {
		s.closeNativeAuthority(room.UUID)
	}
	return s.flushHumanPvp(c), nil
}

func humanForfeitEndPushes(r *HumanPvpRoom) []Push {
	if r.Reason != "player_forfeit" {
		return nil
	}
	return []Push{
		battleSync("set_winner_eid_list", []any{ObjectID(r.Winner)}),
		battleSync("real_battle_end"),
	}
}
