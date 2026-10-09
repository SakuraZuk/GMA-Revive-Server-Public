package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"hs-server/internal/nativeengine"
)

// 单端PVP与真人房间使用同一原生权威与播放契约。防守者没有Client，
// 但仍有冻结身份和阵容；真实防守者的积分由原有双角色事务结算。
func (s *Service) markNativeSoloSession(b *BattleSession) {
	if s.nativePvpConfigured() {
		b.Extra["server_authoritative_pvp"] = true
	}
}

func nativeSoloRequired(b *BattleSession) bool {
	return b != nil && b.Extra["server_authoritative_pvp"] == true
}

func (s *Service) nativeSoloLoadExtra(p Progress, extra map[string]any) map[string]any {
	if !nativeSoloRequired(p.Battle) {
		return extra
	}
	extra["server_authoritative_pvp"] = true
	generation := int64(1)
	if r := p.Battle.NativeSolo; r != nil {
		generation = r.Native.Clients[r.IDs[0]].Generation
	}
	extra["server_authority_generation"] = generation
	return extra
}

func nativeSoloResultExtra(b *BattleSession, extra map[string]any) map[string]any {
	if b != nil && b.NativeSolo != nil {
		extra["client_authoritative"], extra["server_authoritative"], extra["verified"] = false, true, true
		extra["authority_digest"] = b.NativeSolo.Native.Journal.Head
	}
	return extra
}

func (s *Service) beginNativeSolo(ctx context.Context, p *Progress, id string) error {
	b := p.Battle
	if !nativeSoloRequired(b) || b.NativeSolo != nil {
		return nil
	}
	if !s.nativePvpConfigured() || b.Layout == nil || len(b.Team) == 0 {
		return errors.New("单端PVP原生运行时或冻结己方阵容缺失")
	}
	var enemy string
	var enemyTeam []string
	var enemyCards []map[string]any
	battleType := 2
	if m := p.AsyncPvp.Match; m != nil && m.UUID == b.UUID {
		enemy, enemyTeam, enemyCards = m.Enemy.Info.EID, m.Enemy.Team, m.Enemy.Cards
		battleType = 3
		b.AutoBattle = p.AsyncPvp.Auto
	} else if m := p.SyncPvpMatch; m != nil && m.BattleUUID == b.UUID {
		enemy = robotAvatarEID(m.Robot.ID)
		for _, templateID := range m.Robot.SyncPvpCards {
			enemyTeam = append(enemyTeam, robotCardUUID(templateID))
		}
		for _, value := range robotCardsWire(m.Robot) {
			card, ok := value.(map[string]any)
			if !ok {
				return errors.New("同步机器人原生卡快照无效")
			}
			enemyCards = append(enemyCards, card)
		}
		b.AutoBattle = p.battlePreferences().AutoBattle
	} else {
		return errors.New("单端PVP冻结匹配归属失配")
	}
	if !validObjectID(id) || !validObjectID(enemy) || id == enemy || len(enemyTeam) == 0 {
		return errors.New("单端PVP参与者或敌方阵容无效")
	}
	seen := map[string]bool{}
	own := []any{}
	mgr := cardMgrPropertiesWithRunes(p.Cards, p.Runes)
	for _, uuid := range b.Team {
		card, ok := mgr[uuid]
		if !ok || !validObjectID(uuid) || seen[uuid] {
			return errors.New("单端PVP己方卡UUID缺失或重复")
		}
		seen[uuid] = true
		own = append(own, card)
	}
	frozen := map[string]map[string]any{}
	for _, card := range enemyCards {
		uuid := fmt.Sprint(card["uuid"])
		if !validObjectID(uuid) || frozen[uuid] != nil || seen[uuid] {
			return errors.New("单端PVP敌方卡UUID缺失或跨双方重复")
		}
		frozen[uuid] = card
	}
	roster := []any{}
	for _, uuid := range enemyTeam {
		if frozen[uuid] == nil || seen[uuid] {
			return errors.New("单端PVP敌方槽位与冻结卡不符")
		}
		seen[uuid] = true
		roster = append(roster, frozen[uuid])
	}
	r := &HumanPvpRoom{UUID: b.UUID, IDs: []string{id, enemy}, Seed: b.Seed, Dungeon: b.DungeonID, BattleID: b.BattleID, Status: "fighting", Created: b.CreatedAt}
	metadata := map[string]any{"avatar_id": id, "enemy_id": enemy, "battle_uuid": b.UUID,
		"dungeon_id": b.DungeonID, "battle_id": b.BattleID, "battle_type": battleType,
		"enemy_auto": true, "enemy_roster": roster,
		"fighting_card_uuids": b.Layout.Fighting, "support_card_uuids": b.Layout.Support,
		"enemy_fighting_card_uuids": enemyTeam, "enemy_support_card_uuids": []string{}}
	if battleType == 3 {
		metadata["asyn_pvp_auto"] = b.AutoBattle
	}
	if err := s.beginNativeRoom(ctx, r, metadata, own); err != nil {
		return err
	}
	delete(r.Native.Clients, enemy)
	b.NativeSolo = r
	return nil
}

func validateNativeSoloTerminal(b *BattleSession, id string) error {
	r := b.NativeSolo
	if r == nil || r.Native == nil || r.UUID != b.UUID || len(r.IDs) != 2 || r.IDs[0] != id {
		return errors.New("单端PVP权威归属失配")
	}
	client, ok := r.Native.Clients[id]
	if !ok || r.Native.Update.Result == nil || len(r.Native.Winners) != 1 ||
		!client.Checkpoint.TerminalMatches(r.Native.Winners, len(client.Checkpoint.Commands)) {
		return errors.New("单端PVP尚未完成原生唯一胜方及全部播放确认")
	}
	return nil
}

// 重试必须是原始已接受result正文，不能只复用旧Sequence和胜方收据。
func nativeResultRetryMatches(b *BattleSession, envelope *battleEnvelope) bool {
	for i := len(b.EventLog) - 1; i >= 0; i-- {
		event := b.EventLog[i]
		if event.Kind == "result" && event.Sequence == envelope.Sequence {
			before, err := json.Marshal(event.Data)
			after, e := json.Marshal(envelope.Data)
			return err == nil && e == nil && bytes.Equal(before, after)
		}
	}
	return false
}

func nativeSoloRoom(c *Connection) *HumanPvpRoom {
	if b := c.SelectedAvatarUnsafe().Progress.Battle; b != nil {
		return b.NativeSolo
	}
	return nil
}

// 投递游标仅是连接缓存。重连使用新Generation重新绑定并完整播放持久Canonical。
func flushNativeSolo(c *Connection) []Push {
	r := nativeSoloRoom(c)
	if r == nil || r.Native == nil {
		return nil
	}
	id := hexOf(selectedOID(c))
	client := r.Native.Clients[id]
	if c.nativeSoloUUID != r.UUID || c.nativeSoloGeneration != client.Generation {
		c.nativeSoloUUID, c.nativeSoloGeneration, c.nativeSoloCursor = r.UUID, client.Generation, 0
	}
	result := []Push{}
	for _, outbound := range r.Native.Outbound {
		if outbound.Index > c.nativeSoloCursor && outbound.Target == id && outbound.Generation == client.Generation {
			result = append(result, nativePlaybackPush(outbound))
			c.nativeSoloCursor = outbound.Index
		}
	}
	return result
}

func (s *Service) nativeSoloDoCommand(ctx context.Context, c *Connection, command string, args []any) (bool, []Push, error) {
	if !nativeSoloRequired(c.SelectedAvatarUnsafe().Progress.Battle) {
		return false, nil, nil
	}
	if command != "move_to" && command != "use_skill" {
		return true, nil, errors.New("单端PVP仅接受原生移动与技能点击")
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.Finished || b.NativeSolo == nil || !b.BridgeStarted {
			return errors.New("单端PVP尚未完成原生加载")
		}
		r := b.NativeSolo
		id := hexOf(selectedOID(c))
		if r.Native.Update.Result != nil || !nativeAllClientsReady(r) {
			return errors.New("单端PVP上一权威动作尚未播放完成")
		}
		client := r.Native.Clients[id]
		click, err := nativeengine.MapClick(command, args, r.Native.Update.State, [2]string{r.IDs[0], r.IDs[1]}, id, client.Mapping, client.Frame)
		if err != nil {
			return err
		}
		return s.advanceNativeUntilBoundary(ctx, r, map[string]any{"operation": "step", "command": click})
	})
	if err != nil {
		return true, nil, err
	}
	return true, flushNativeSolo(c), nil
}

func (s *Service) absorbNativeSoloEvent(ctx context.Context, c *Connection, envelope *battleEnvelope) (bool, []Push, error) {
	snapshot := c.SelectedAvatarUnsafe().Progress
	if !nativeSoloRequired(snapshot.Battle) {
		return false, nil, nil
	}
	if snapshot.Battle.UUID != envelope.BattleUUID {
		return true, nil, errors.New("单端PVP事件UUID失配")
	}
	// 本版prepare先报告ready，然后才上报battle_fighting阵容。
	// 允许这一次早握手；任何started/动作/result仍须先冻结并启动原生。
	if snapshot.Battle.NativeSolo == nil {
		err := s.updateProgress(ctx, c, func(p *Progress) error {
			b := p.Battle
			if b == nil || b.UUID != envelope.BattleUUID || b.Finished || envelope.Kind != "ready" || envelope.Generation != 1 || envelope.Data["bridge_revision"] != float64(31) {
				return errors.New("单端PVP尚未冻结原生阵容，仅允许原生桥早握手")
			}
			if err := acceptBattleEvent(b, envelope); err != nil {
				return err
			}
			nativeRecordBattleEvent(b, envelope)
			return nil
		})
		return true, nil, err
	}
	if envelope.Kind == "result" && snapshot.AsyncPvp.Match != nil && snapshot.AsyncPvp.Match.UUID == envelope.BattleUUID {
		handled, pushes, err := s.absorbAsyncPvpResult(ctx, c, envelope)
		if err == nil && handled {
			s.closeNativeAuthority(envelope.BattleUUID)
		}
		return handled, pushes, err
	}
	var receipt *SyncPvpResult
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.UUID != envelope.BattleUUID || b.NativeSolo == nil {
			return errors.New("单端PVP持久原生会话缺失")
		}
		r := b.NativeSolo
		id := hexOf(selectedOID(c))
		client := r.Native.Clients[id]
		if b.Finished {
			if envelope.Kind != "result" || envelope.Generation != client.Generation || client.Checkpoint.ResultCandidate == nil || envelope.Sequence != client.Checkpoint.ResultCandidate.Sequence {
				return errors.New("单端PVP已结束，只接受原末态重试")
			}
			if !nativeResultRetryMatches(b, envelope) {
				return errors.New("单端PVP末态重试正文被更改")
			}
			if err := validateNativeSoloTerminal(b, id); err != nil {
				return err
			}
			if got, ok := p.SyncPvpSettlements[b.UUID]; ok {
				result := got.Result
				receipt = &result
				return nil
			}
			return errors.New("单端同步PVP收据缺失")
		}
		if err := s.nativeObservePlayback(ctx, r, b, id, envelope); err != nil {
			return err
		}
		if envelope.Kind == "settings" {
			b.AutoBattle, _ = envelope.Data["auto_battle"].(bool)
			if p.AsyncPvp.Match != nil && p.AsyncPvp.Match.UUID == b.UUID {
				p.AsyncPvp.Auto = b.AutoBattle
			} else {
				prefs := p.battlePreferences()
				prefs.AutoBattle = b.AutoBattle
				p.BattlePreferences = prefs
			}
		}
		if envelope.Kind != "result" {
			return nil
		}
		if err := validateNativeSoloTerminal(b, id); err != nil {
			return err
		}
		b.Outcome = "loss"
		if r.Native.Winners[0] == id {
			b.Outcome = "win"
		}
		b.WinnerEIDs, b.Finished, b.Status = append([]string(nil), r.Native.Winners...), true, "结束"
		result, err := settleSyncPvpResult(p, 0, s.Now())
		if err != nil {
			return err
		}
		r.Status, r.Winner, r.FinalDigest = "settled", r.Native.Winners[0], r.Native.Journal.Head
		receipt = &result
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	if receipt != nil {
		s.closeNativeAuthority(envelope.BattleUUID)
		return true, syncPvpResultPushes(c, *receipt), nil
	}
	return true, flushNativeSolo(c), nil
}

func (s *Service) tickNativeSolo(ctx context.Context, c *Connection) []Push {
	r := nativeSoloRoom(c)
	if r == nil || c.SelectedAvatarUnsafe().Progress.Battle.Finished {
		return nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil || b.NativeSolo == nil || b.Finished {
			return nil
		}
		r := b.NativeSolo
		if r.Native.Update.Result != nil || !r.Native.Update.State.AwaitingPlayer || !nativeAllClientsReady(r) {
			return nil
		}
		if !b.AutoBattle {
			now := s.Now().Unix()
			if r.Native.WindowDeadline == 0 {
				r.Native.WindowDeadline = now + nativeDeadlineSeconds(r.Native.Update)
				return nil
			}
			if now < r.Native.WindowDeadline {
				return nil
			}
		}
		return s.advanceNativeUntilBoundary(ctx, r, map[string]any{"operation": "timeout"})
	})
	if err != nil {
		log.Printf("单端PVP原生推进失败，保留已提交Journal重试：%v", err)
		return nil
	}
	return flushNativeSolo(c)
}

func (s *Service) resumeNativeSolo(ctx context.Context, c *Connection) (bool, []Push, error) {
	if !nativeSoloRequired(c.SelectedAvatarUnsafe().Progress.Battle) {
		return false, nil, nil
	}
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		b := p.Battle
		if b == nil {
			return errors.New("单端PVP恢复状态缺失")
		}
		if b.NativeSolo == nil {
			if b.Finished {
				return errors.New("单端PVP缺少原生结算收据")
			}
			b.BridgeReady, b.BridgeStarted, b.Loaded, b.Started = false, false, false, false
			b.LastSequence, b.EventLog = 0, nil
			return nil
		}
		if b.Finished {
			return nil
		}
		r := b.NativeSolo
		id := hexOf(selectedOID(c))
		client := r.Native.Clients[id]
		client.Generation++
		client.Mapping, client.Frame = map[string]string{}, nil
		client.Checkpoint = nativeengine.Checkpoint{Generation: client.Generation}
		client.CanonicalCursor, client.WindowHead = 0, ""
		r.Native.Clients[id] = client
		// 单端旧世代描述已不可能再投递；新世代由Canonical完整重建，防恢复反复膨胀。
		r.Native.Outbound = nil
		r.Native.WindowDeadline = 0
		b.BridgeReady, b.BridgeStarted, b.Loaded, b.Started = false, false, false, false
		b.LastSequence, b.EventLog = 0, nil
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	p := c.SelectedAvatarUnsafe().Progress
	b := p.Battle
	if b.Finished {
		if m := p.AsyncPvp.Match; m != nil && m.UUID == b.UUID && m.Receipt != nil {
			return true, asyncResultPushes(c, *m.Receipt, m.Rewards), nil
		}
		if receipt, ok := p.SyncPvpSettlements[b.UUID]; ok {
			return true, syncPvpResultPushes(c, receipt.Result), nil
		}
		return true, nil, errors.New("单端PVP恢复缺少原收据")
	}
	c.battleStartSent = false
	if m := p.AsyncPvp.Match; m != nil && m.UUID == b.UUID {
		return true, s.asyncPvpLoadPushes(c, m), nil
	}
	return true, s.syncPVPLoadPushes(c, p.SyncPvpMatch, 0, false), nil
}
