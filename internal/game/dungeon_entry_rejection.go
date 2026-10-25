package game

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// 原版 dungeon_mgr.enter_dungeon.server_callback 只接收 ret。
// 业务拒绝必须结束该请求的等待；数据库及协议错误继续交给上层处理。
func (s *Service) enterDungeonRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	out, err := s.enterDungeon(ctx, c, args)
	if err == nil {
		return out, nil
	}
	var rejected *runeBusinessError
	var cb int
	if c.phase != Playing || len(args) != 3 || json.Unmarshal(args[0], &cb) != nil || !errors.As(err, &rejected) {
		return nil, err
	}
	code, ok := androidShop.Errors[rejected.Name]
	if !ok || code == RetSuccess {
		return nil, err
	}
	return []Push{Callback(cb, []any{code})}, nil
}

// 条件不满足是可显示的入场拒绝，规则无法解析则保留诊断错误。
func ordinaryDungeonConditions(row activityRow, p Progress, now time.Time) error {
	if len(row["unlock_condition"]) == 0 {
		return nil
	}
	var conditions [][][]json.RawMessage
	if json.Unmarshal(row["unlock_condition"], &conditions) != nil {
		return errors.New("副本条件结构无效")
	}
	ok, err := shopConditions(conditions, p, p.AvatarLevel, now)
	if err != nil {
		return err
	}
	if !ok {
		return runeReject("RET_DUNGEON_LOCKED", "副本前置条件未满足")
	}
	return nil
}
