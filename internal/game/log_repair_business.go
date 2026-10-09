package game

import (
	"context"
	"encoding/json"
	"errors"
	"log"
)

// 原生widgets.layout_lock_button使用整数0/1；只兼容这两值，不做任意真值转换。
func nativeBinarySwitch(raw json.RawMessage, value *bool) bool {
	if string(raw) == "null" {
		return false
	}
	if json.Unmarshal(raw, value) == nil {
		return true
	}
	var n int
	if json.Unmarshal(raw, &n) != nil || n < 0 || n > 1 {
		return false
	}
	*value = n == 1
	return true
}

// 0EC45730和6562F4CC：仅type4/stype9，原表target_id为单件灵感量；回调为增量、消息两参。
func (s *Service) consumePowerMaterialRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	cb, valid := callbackArg(args)
	if c.phase != Playing || !valid || len(args) != 3 {
		return nil, errors.New("灵感材料需要已登录角色及回调、材料、数量")
	}
	var id int
	var count int64
	added := int64(0)
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if json.Unmarshal(args[1], &id) != nil || json.Unmarshal(args[2], &count) != nil || count <= 0 {
			return errors.New("灵感材料参数无效")
		}
		row, exists := androidShop.Materials[id]
		if !exists || row.Type != 4 || row.Sub != 9 || row.Target <= 0 {
			return errors.New("该材料不是原生灵感材料")
		}
		// 原生拒绝after_add>=9999；先除后乘避免数量溢出。
		p.settlePowerRecovery(s.Now())
		if p.Power.Value < 0 || p.Power.Value >= 9999 || count > int64(9998-p.Power.Value)/int64(row.Target) {
			return errors.New("灵感达到原生9999上限")
		}
		material := p.Materials[id]
		if material.Count < count {
			return errors.New("灵感材料余额不足")
		}
		added = int64(row.Target) * count
		material.Count -= count
		p.Materials[id] = material
		p.Power.Value += int(added)
		return nil
	})
	if err != nil {
		log.Printf("灵感材料拒绝 uid=%d 原因=%v", c.SelectedAvatarUnsafe().UID, err)
		return []Push{Callback(cb, []any{0, "灵感材料使用失败"})}, nil
	}
	return []Push{materialManagerPush(c), powerPush(c), Callback(cb, []any{added, ""})}, nil
}

// BB2B671E的锁定/解锁是单参IntRet回调，卡字段lock由原生归还检查读取。
func (s *Service) cardLockRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	cb, valid := callbackArg(args)
	if c.phase != Playing || !valid || len(args) != 2 {
		return nil, errors.New("幻书锁定参数数量或回调无效")
	}
	var uuid string
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if json.Unmarshal(args[1], &uuid) != nil {
			return errors.New("幻书锁定标识无效")
		}
		_, card := findCard(p, uuid)
		if card == nil {
			return errors.New("幻书不属于当前角色")
		}
		card.Lock = 0
		if method == "lock_card" {
			card.Lock = 1
		}
		return nil
	})
	if err != nil {
		return []Push{Callback(cb, []any{1})}, nil
	}
	return []Push{cardMgrPush(c), Callback(cb, []any{RetSuccess})}, nil
}

// 普通退出走真实失败奖励及返还账本；真人房间继续双角色弃权事务。
func (s *Service) exitBattleRPC(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 0 {
		return nil, errors.New("退出战斗需要已登录角色且不接受参数")
	}
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	if p.Social.HumanRoom != nil && b != nil && b.UUID == p.Social.HumanRoom.UUID {
		return s.humanExitBattle(ctx, c, args)
	}
	if b != nil && (b.NativeSolo != nil || p.SyncPvpMatch != nil || p.AsyncPvp.Match != nil && p.AsyncPvp.Match.UUID == b.UUID) {
		return nil, errors.New("单端竞技退出须经原生权威弃权，不能按普通副本结算")
	}
	// 已收到显式末态但SQL提交延后时，不能用退出覆盖胜负。
	if c.pendingOrdinaryResult != nil {
		out, err := s.retryOrdinaryResult(ctx, c)
		if err != nil || c.pendingOrdinaryResult != nil {
			return out, err
		}
		// 真实末态可能已自动建立排队的下一副本，不能把下一战斗误关掉。
		if next := c.SelectedAvatarUnsafe().Progress.Battle; next != nil && !next.Finished {
			return out, nil
		}
		return append(out, push("Avatar", "exit_battle_ok")), nil
	}
	settled := false
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.Finished {
			return nil
		}
		if b.NativeSolo != nil || p.Social.HumanRoom != nil && p.Social.HumanRoom.UUID == b.UUID || p.SyncPvpMatch != nil || p.AsyncPvp.Match != nil && p.AsyncPvp.Match.UUID == b.UUID {
			return errors.New("退出事务发现竞技会话，拒绝普通结算")
		}
		var box map[string]any
		var err error
		if b.ActivityContext != nil {
			box, err = settleActivityDungeon(p, b, false, nil, s.Now())
		} else {
			box, err = settleOrdinaryDungeonRewards(p, b, false, s.Now())
		}
		if err != nil {
			return err
		}
		if b.ActivityContext != nil {
			if err := applyBattleCardExp(p, b, box); err != nil {
				return err
			}
		}
		if err := settleRemainingDungeon(p, b, false, nil, box, s.Now()); err != nil {
			return err
		}
		b.Finished, b.Status, b.Outcome = true, "结束", "loss"
		b.WinnerEIDs, b.SettlementBox = []string{}, box
		settled = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []Push{materialManagerPush(c), powerPush(c)}
	// 已排队的下一入口也必须释放其真实回调，不能清缓存后留下永久等待。
	if cb, ok := callbackArg(c.pendingDungeonArgs); ok {
		values := []any{1}
		if c.pendingDungeonBoxCallback {
			values = append(values, nil)
		}
		out = append(out, Callback(cb, values))
	}
	c.ordinaryObservation, c.pendingOrdinaryResult = nil, nil
	c.pendingDungeonArgs, c.pendingBattleFightingArgs = nil, nil
	c.pendingDungeonBoxCallback = false
	c.pendingBattleFightingUUID, c.ordinaryPrepareUUID = "", ""
	c.battleStartSent = false
	out = append(out, push("Avatar", "exit_battle_ok"))
	// 原生退出确认只调用real_battle_end，桥随后上报result；数据库已结束
	// 会拒绝该上报。必须在确认之后发送已持久的失败结果，释放结果界面等待。
	// 重复退出只重发确认，不重复结算或重复弹出结果界面。
	if settled {
		b := c.SelectedAvatarUnsafe().Progress.Battle
		result, err := (&battleSettlement{outcome: b.Outcome, dungeonID: b.DungeonID, fullBonus: b.SettlementBox, settledAt: s.Now()}).pushes(c)
		if err != nil {
			return nil, err
		}
		out = append(out, result...)
	}
	return out, nil
}
