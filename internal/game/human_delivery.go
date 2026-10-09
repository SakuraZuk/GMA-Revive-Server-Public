package game

import (
	"bytes"
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// HumanDelivery仅保存原生动作描述符；与房间镜像同事务提交，跨进程读取时恢复typed wire。
// 投递不是第二份战斗权威，唯一UUID、seed、命令序号及结果仍由HumanPvpRoom决定。
type HumanDelivery struct {
	Sequence     int64  `json:"sequence"`
	Target       string `json:"target"`
	Kind         string `json:"kind"`
	CommandIndex int    `json:"command_index,omitempty"`
	Replay       bool   `json:"replay,omitempty"`
}
type HumanRefusal struct {
	Sequence int64         `json:"sequence"`
	Profile  SocialProfile `json:"profile"`
	Reason   int           `json:"reason"`
	Created  int64         `json:"created"`
}

// Handle在任何玩家事务开始前调用；不在事务回调内重读或覆盖未提交状态。
func humanConnectionMethod(method string) bool {
	switch method {
	case "exit_battle", "leave_battle", "quit_battle":
		return true
	case "challenge_friend", "agree_challenge", "cancel_challenge", "refuse_challenge", "send_pvp_cards", "pvp_load_complete", "battle_fighting", "do_command", "client_need_recover_battle", "start_sync_pvp_match", "cancel_sync_pvp_match", "get_record_result", "start_record_battle", "upload_native_battle_record":
		return true
	}
	return false
}

func appendHumanDelivery(r *HumanPvpRoom, id, kind string, index int, replay bool) {
	r.Deliveries = append(r.Deliveries, HumanDelivery{Sequence: int64(len(r.Deliveries)) + 1, Target: id, Kind: kind, CommandIndex: index, Replay: replay})
}

// 任一事务失败会同时撤销房间变化和投递；网络重试不生成第二个动作。
func recordHumanTransitions(before, r *HumanPvpRoom) error {
	if len(r.Commands) < len(before.Commands) {
		return errors.New("真人共同命令日志不能缩短")
	}
	for i, old := range before.Commands {
		if !bytes.Equal(mustJSON(old), mustJSON(r.Commands[i])) {
			return errors.New("真人既有命令日志不能覆盖")
		}
	}
	if before.Native != nil {
		if r.Native == nil || len(r.Native.Canonical) < len(before.Native.Canonical) || len(r.Native.Outbound) < len(before.Native.Outbound) {
			return errors.New("PVP原生权威日志不能移除")
		}
		for i, old := range before.Native.Canonical {
			if !bytes.Equal(mustJSON(old), mustJSON(r.Native.Canonical[i])) {
				return errors.New("PVP既有规范命令日志不能覆盖")
			}
		}
		for i, old := range before.Native.Outbound {
			if !bytes.Equal(mustJSON(old), mustJSON(r.Native.Outbound[i])) {
				return errors.New("PVP既有客户端投递不能覆盖")
			}
		}
	}
	if before.Status == "select" && r.Status == "loading" {
		for _, id := range r.IDs {
			appendHumanDelivery(r, id, "load", 0, false)
		}
	}
	for _, id := range r.IDs {
		if !before.Players[id].Started && r.Players[id].Started {
			_, replay := r.Recovering[id]
			appendHumanDelivery(r, id, "start", 0, replay)
		}
	}
	for _, cmd := range r.Commands[len(before.Commands):] {
		for _, id := range r.IDs {
			appendHumanDelivery(r, id, "command", cmd.Index, false)
		}
	}
	oldOutbound := 0
	if before.Native != nil {
		oldOutbound = len(before.Native.Outbound)
	}
	if r.Native != nil {
		for _, outbound := range r.Native.Outbound[oldOutbound:] {
			appendHumanDelivery(r, outbound.Target, "native_command", outbound.Index, false)
		}
	}
	if before.Status != "settled" && r.Status == "settled" {
		for _, id := range r.IDs {
			appendHumanDelivery(r, id, "result", 0, false)
		}
	}
	if before.Status != "aborted" && r.Status == "aborted" {
		for _, id := range r.IDs {
			appendHumanDelivery(r, id, "abort", 0, false)
		}
	}
	if len(r.Deliveries) > 8192 {
		return errors.New("真人持久投递达到保护上限，事务未提交")
	}
	return nil
}

func humanDeliveryPushes(r *HumanPvpRoom, d HumanDelivery) ([]Push, error) {
	if _, ok := r.Players[d.Target]; !ok {
		return nil, errors.New("真人投递目标不属于共同房间")
	}
	switch d.Kind {
	case "select":
		return []Push{humanSelection(r)}, nil
	case "load":
		return humanLoadingPushes(r, d.Target), nil
	case "start":
		out := humanStartPushes(r)
		if d.Replay && r.Native == nil {
			out = append([]Push{humanReplayPush(r)}, out...)
		}
		return out, nil
	case "command":
		if d.CommandIndex < 1 || d.CommandIndex > len(r.Commands) {
			return nil, errors.New("真人持久投递命令序号失配")
		}
		return []Push{humanCommandPush(r, r.Commands[d.CommandIndex-1])}, nil
	case "native_command":
		if r.Native == nil || d.CommandIndex < 1 || d.CommandIndex > len(r.Native.Outbound) {
			return nil, errors.New("PVP原生持久投递序号失配")
		}
		outbound := r.Native.Outbound[d.CommandIndex-1]
		if outbound.Target != d.Target || outbound.Index != d.CommandIndex {
			return nil, errors.New("PVP原生持久投递目标或索引失配")
		}
		return []Push{nativePlaybackPush(outbound)}, nil
	case "result":
		return humanResultPushes(r, d.Target), nil
	case "abort":
		return humanAbortPushes(), nil
	default:
		return nil, errors.New("真人持久投递类型未知")
	}
}

// 调用方持有本连接锁；只读最新数据库，不取得其他连接锁。
// 返回同一SQL快照的角色元数据与进度版本；版本未变时不传输/解析整份JSONB。
type HumanAvatarSnapshotAccounts interface {
	HumanAvatarSnapshot(context.Context, []byte, int64) (Avatar, int64, bool, error)
}

func (s *Service) refreshHumanConnection(ctx context.Context, c *Connection) error {
	if c.phase != Playing {
		return nil
	}
	if store, ok := s.Accounts.(HumanAvatarSnapshotAccounts); ok {
		oid := selectedOID(c)
		revision := int64(-1)
		if c.humanSnapshotKnown && c.humanSnapshotOID == string(oid) {
			revision = c.humanSnapshotRevision
		}
		av, current, changed, err := store.HumanAvatarSnapshot(ctx, oid, revision)
		if err != nil {
			return err
		}
		for i, old := range c.identity.Avatars {
			if old.Hostnum == c.hostnum {
				av.Account = old.Account
				if !changed {
					av.Progress = old.Progress
				}
				av.Progress.AvatarLevel = av.Info.Level
				mergeOrdinaryObservation(av.Progress.Battle, c.ordinaryObservation)
				c.identity.Avatars[i] = av
			}
		}
		c.humanSnapshotKnown, c.humanSnapshotOID, c.humanSnapshotRevision = true, string(oid), current
		return nil
	}
	if store, ok := s.Accounts.(AdminAccounts); ok {
		av, err := store.AdminPlayer(ctx, selectedOID(c))
		if err != nil {
			return err
		}
		for i, old := range c.identity.Avatars {
			if old.Hostnum == c.hostnum {
				av.Account = old.Account
				mergeOrdinaryObservation(av.Progress.Battle, c.ordinaryObservation)
				c.identity.Avatars[i] = av
			}
		}
		return nil
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil
	}
	rows, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{hexOf(selectedOID(c))}, Limit: 1})
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return errors.New("真人投递角色不存在")
	}
	for i, old := range c.identity.Avatars {
		if old.Hostnum == c.hostnum {
			rows[0].Account = old.Account
			rows[0].Gender = old.Gender
			rows[0].NicknameSet = old.NicknameSet
			rows[0].CreatedAt = old.CreatedAt
			c.identity.Avatars[i] = rows[0]
		}
	}
	return nil
}

func (s *Service) flushHumanPvp(c *Connection) []Push {
	if c.phase != Playing {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.refreshHumanConnection(ctx, c); err != nil {
		log.Printf("真人持久投递读取失败，游标未推进：%v", err)
		return nil
	}
	return s.flushHumanPvpSnapshot(c)
}

// Tick已经在tickHumanPvp读最新存档；此分支避免同Tick重复读整份JSONB。
func (s *Service) flushHumanPvpSnapshot(c *Connection) []Push {
	p := c.SelectedAvatarUnsafe().Progress
	out := []Push{}
	for _, notice := range p.Social.HumanRefusals {
		if notice.Sequence > c.humanRefusalSequence {
			if s.Now().Unix()-notice.Created <= int64(socialCatalog.Constants["CHALLENGE_WAIT_TIME"]) {
				out = append(out, push("Avatar", "on_refuse_challenge", s.socialInfo(notice.Profile), notice.Reason))
			}
			c.humanRefusalSequence = notice.Sequence
		}
	}
	r := p.Social.HumanRoom
	if r == nil {
		return out
	}
	if c.humanDeliveryUUID != r.UUID {
		c.humanDeliveryUUID = r.UUID
		c.humanDeliverySequence = 0
	}
	id := hexOf(selectedOID(c))
	sequence := c.humanDeliverySequence
	for _, d := range r.Deliveries {
		if d.Sequence <= sequence {
			continue
		}
		if d.Target == id {
			pushes, err := humanDeliveryPushes(r, d)
			if err != nil {
				log.Printf("真人持久投递校验失败，游标未推进：%v", err)
				return out
			}
			out = append(out, pushes...)
		}
		sequence = d.Sequence
	}
	c.humanDeliverySequence = sequence
	return out
}

// 新登录不弹出旧拒绝通知；旧已结算房间只在显式recover中补收据。
func (s *Service) initializeHumanDelivery(c *Connection) {
	p := c.SelectedAvatarUnsafe().Progress
	if n := len(p.Social.HumanRefusals); n > 0 {
		c.humanRefusalSequence = p.Social.HumanRefusals[n-1].Sequence
	}
	if r := p.Social.HumanRoom; r != nil && (r.Status == "settled" || r.Status == "aborted") {
		c.humanDeliveryUUID = r.UUID
		c.humanDeliverySequence = int64(len(r.Deliveries))
	}
}

// 20秒租约和5秒刷新是本服跨进程在线政策；多连接分开保存，不能互删其他设备租约。
type HumanPresenceAccounts interface {
	RefreshHumanPresence(context.Context, []byte, string, int64) error
	RemoveHumanPresence(context.Context, string) error
	HumanOnlineIDs(context.Context, int64) ([]string, error)
}

func (s *Service) refreshHumanPresence(ctx context.Context, c *Connection) error {
	store, ok := s.Accounts.(HumanPresenceAccounts)
	if !ok || c.phase != Playing {
		return nil
	}
	now := s.Now().Unix()
	if now < c.nextHumanPresenceRefresh && c.humanPresenceID != "" {
		return nil
	}
	if c.humanPresenceID == "" {
		c.humanPresenceID = newBattleUUID(s.Now())
	}
	if err := store.RefreshHumanPresence(ctx, selectedOID(c), c.humanPresenceID, now+20); err != nil {
		// 失败仍保留原租约期限，稍后重试，避免每100毫秒争抢数据库。
		c.nextHumanPresenceRefresh = s.Now().Unix() + 1
		return err
	}
	c.nextHumanPresenceRefresh = now + 5
	_, err := s.humanOnlineIDs(ctx)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) removeHumanPresence(c *Connection) {
	store, ok := s.Accounts.(HumanPresenceAccounts)
	if !ok || c.humanPresenceID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.RemoveHumanPresence(ctx, c.humanPresenceID); err != nil {
		log.Printf("真人离线租约清理失败，等待租约过期：%v", err)
	}
	_, _ = s.humanOnlineIDs(ctx)
}
func (s *Service) humanOnlineIDs(ctx context.Context) (map[string]bool, error) {
	ids := map[string]bool{}
	remote := map[string]bool{}
	s.onlineMu.Lock()
	for id, connections := range s.online {
		if len(connections) > 0 {
			ids[id] = true
		}
	}
	s.onlineMu.Unlock()
	if store, ok := s.Accounts.(HumanPresenceAccounts); ok {
		list, err := store.HumanOnlineIDs(ctx, s.Now().Unix())
		if err != nil {
			return nil, err
		}
		for _, id := range list {
			ids[id] = true
			remote[id] = true
		}
	}
	humanOnlineSnapshots.Store(s, humanOnlineSnapshot{IDs: remote, Expires: s.Now().Unix() + 5})
	return ids, nil
}

type humanOnlineSnapshot struct {
	IDs     map[string]bool
	Expires int64
}

var humanOnlineSnapshots sync.Map

func (s *Service) humanKnownOnline() map[string]bool {
	ids := map[string]bool{}
	if value, ok := humanOnlineSnapshots.Load(s); ok {
		snapshot := value.(humanOnlineSnapshot)
		if snapshot.Expires > s.Now().Unix() {
			for id, online := range snapshot.IDs {
				ids[id] = online
			}
		}
	}
	s.onlineMu.Lock()
	for id, cs := range s.online {
		if len(cs) > 0 {
			ids[id] = true
		}
	}
	s.onlineMu.Unlock()
	return ids
}
