package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"hs-server/internal/nativeengine"
)

const nativePvpRoomVersion = 1

// NativePvpRoom是共同房间内唯一持久权威。进程只是可丢弃缓存；Journal、
// 原生命令和每端播放屏障与双方Progress在同一数据库事务内提交。
type NativePvpRoom struct {
	Version        int                         `json:"version"`
	Journal        nativeengine.Journal        `json:"journal"`
	Update         nativeengine.Update         `json:"update"`
	InitialUnits   []nativeengine.UnitSnapshot `json:"initial_units"`
	Clients        map[string]NativePvpClient  `json:"clients"`
	Canonical      []NativePvpCanonical        `json:"canonical,omitempty"`
	Outbound       []NativePvpOutbound         `json:"outbound,omitempty"`
	Winners        []string                    `json:"winners,omitempty"`
	Activated      bool                        `json:"activated,omitempty"`
	WindowDeadline int64                       `json:"window_deadline,omitempty"`
	ResultDeadline int64                       `json:"result_deadline,omitempty"`
}

type NativePvpClient struct {
	Generation      int64                         `json:"generation"`
	Mapping         map[string]string             `json:"mapping,omitempty"`
	Frame           *nativeengine.CoordinateFrame `json:"frame,omitempty"`
	Checkpoint      nativeengine.Checkpoint       `json:"checkpoint"`
	CanonicalCursor int                           `json:"canonical_cursor,omitempty"`
	WindowHead      string                        `json:"window_head,omitempty"`
}

type NativePvpCanonical struct {
	State nativeengine.State `json:"state"`
	Event nativeengine.Event `json:"event"`
}

// NativePvpOutbound保存可重建的客户端RPC描述，不保存连接或进程对象。
type NativePvpOutbound struct {
	Index      int                   `json:"index"`
	Target     string                `json:"target"`
	Generation int64                 `json:"generation"`
	Playback   nativeengine.Playback `json:"playback"`
	Window     bool                  `json:"window,omitempty"`
}

func (s *Service) loadNativePvpEnvironment() {
	python := strings.TrimSpace(os.Getenv("HS_NATIVE_PVP_PYTHON"))
	worker := strings.TrimSpace(os.Getenv("HS_NATIVE_PVP_WORKER"))
	directory := strings.TrimSpace(os.Getenv("HS_NATIVE_PVP_DIRECTORY"))
	resource := strings.TrimSpace(os.Getenv("HS_NATIVE_PVP_RESOURCE_SHA256"))
	if python == "" && worker == "" && directory == "" && resource == "" {
		return
	}
	if python == "" || worker == "" || directory == "" || resource == "" {
		log.Printf("PVP单原生权威配置不完整，真人房间保持旧路线")
		return
	}
	config := nativeengine.Config{Python: python, Worker: worker, Directory: directory}
	if probe, err := nativeengine.NewAuthority(config, resource); err != nil {
		log.Printf("PVP单原生权威资源配置无效：%v", err)
		return
	} else {
		probe.Close()
	}
	s.nativePvpConfig, s.nativePvpResource = &config, resource
}

func (s *Service) nativePvpConfigured() bool {
	return s.nativePvpConfig != nil && s.nativePvpResource != ""
}

func (s *Service) nativeAuthority(uuid string) (*nativeengine.Authority, error) {
	if !s.nativePvpConfigured() || !validObjectID(uuid) {
		return nil, errors.New("PVP单原生权威未配置或房间无效")
	}
	s.nativePvpMu.Lock()
	defer s.nativePvpMu.Unlock()
	if authority := s.nativePvpAuthorities[uuid]; authority != nil {
		return authority, nil
	}
	authority, err := nativeengine.NewAuthority(*s.nativePvpConfig, s.nativePvpResource)
	if err != nil {
		return nil, err
	}
	s.nativePvpAuthorities[uuid] = authority
	return authority, nil
}

func (s *Service) closeNativeAuthority(uuid string) {
	s.nativePvpMu.Lock()
	authority := s.nativePvpAuthorities[uuid]
	delete(s.nativePvpAuthorities, uuid)
	s.nativePvpMu.Unlock()
	if authority != nil {
		authority.Close()
	}
}

func (s *Service) beginNativePvp(ctx context.Context, r *HumanPvpRoom) error {
	metadata, roster, err := nativePvpPayload(r)
	if err != nil {
		return err
	}
	return s.beginNativeRoom(ctx, r, metadata, roster)
}

func (s *Service) beginNativeRoom(ctx context.Context, r *HumanPvpRoom, metadata map[string]any, roster []any) error {
	authority, err := s.nativeAuthority(r.UUID)
	if err != nil {
		return err
	}
	journal, raw, err := authority.Begin(ctx, map[string]any{
		"operation": "start", "metadata": metadata, "roster": roster,
		"seed": r.Seed, "auto": false,
	})
	if err != nil {
		return err
	}
	update, err := nativeengine.ParseUpdate(raw)
	if err != nil {
		return err
	}
	native := &NativePvpRoom{Version: nativePvpRoomVersion, Journal: *journal, Update: *update, InitialUnits: append([]nativeengine.UnitSnapshot(nil), update.State.Units...), Clients: map[string]NativePvpClient{}}
	for _, id := range r.IDs {
		native.Clients[id] = NativePvpClient{Generation: 1, Mapping: map[string]string{}, Checkpoint: nativeengine.Checkpoint{Generation: 1}}
	}
	r.Native = native
	return nil
}

func nativeUnitSnapshots(value any) ([]nativeengine.UnitSnapshot, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var units []nativeengine.UnitSnapshot
	if err = json.Unmarshal(raw, &units); err != nil || len(units) == 0 || len(units) > battleRosterMax {
		return nil, errors.New("权威PVP客户端单位快照无效")
	}
	return units, nil
}

func nativePlaybackPush(out NativePvpOutbound) Push {
	args := append([]any(nil), out.Playback.Args...)
	if out.Playback.Method == "revival_do_peer_command" {
		args = append([]any{ObjectID(out.Playback.Master)}, args...)
	}
	return battleSync(out.Playback.Method, args...)
}

func nativeAllClientsStarted(r *HumanPvpRoom) bool {
	if r.Native == nil {
		return false
	}
	for _, id := range nativeRecipients(r) {
		client, ok := r.Native.Clients[id]
		if !ok || client.Frame == nil || client.Checkpoint.StartedSequence == 0 {
			return false
		}
	}
	return true
}

func nativeAllClientsReady(r *HumanPvpRoom) bool {
	if !nativeAllClientsStarted(r) {
		return false
	}
	for _, id := range nativeRecipients(r) {
		client := r.Native.Clients[id]
		if client.CanonicalCursor != len(r.Native.Canonical) || !client.Checkpoint.Ready() {
			return false
		}
	}
	return true
}

func nativeCanonicalState(update *nativeengine.Update, event nativeengine.Event) nativeengine.State {
	wanted := map[string]bool{}
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case string:
			wanted[item] = true
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(event.Data)
	state := nativeengine.State{}
	for _, unit := range update.State.Units {
		if wanted[fmt.Sprint(unit["eid"])] {
			state.Units = append(state.Units, unit)
		}
	}
	return state
}

func (s *Service) appendNativeCanonical(r *HumanPvpRoom, update *nativeengine.Update) error {
	for _, event := range update.Events {
		if event.Kind != "sync_command" {
			continue
		}
		canonical := NativePvpCanonical{State: nativeCanonicalState(update, event), Event: event}
		r.Native.Canonical = append(r.Native.Canonical, canonical)
		if len(r.Native.Canonical) > nativeengine.MaxJournalEntries {
			return errors.New("PVP规范播放日志达到保护上限")
		}
		if encoded, _ := json.Marshal(r.Native.Canonical); len(encoded) > nativeengine.MaxJournalBytes {
			return errors.New("PVP规范播放日志超过持久保护上限")
		}
		for _, id := range nativeRecipients(r) {
			client := r.Native.Clients[id]
			if client.Frame == nil {
				continue
			}
			if err := flushNativePlaybacks(r, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func flushNativePlaybacks(r *HumanPvpRoom, id string) error {
	client := r.Native.Clients[id]
	for client.CanonicalCursor < len(r.Native.Canonical) {
		canonical := r.Native.Canonical[client.CanonicalCursor]
		play, err := nativeengine.MapPlayback(canonical.Event, canonical.State, [2]string{r.IDs[0], r.IDs[1]}, id, client.Mapping, client.Frame)
		if errors.Is(err, nativeengine.ErrUnmappedEntity) {
			return nil
		}
		if err != nil {
			return err
		}
		client.CanonicalCursor++
		r.Native.Clients[id] = client
		if play == nil {
			continue
		}
		eid, raw, round, err := nativeengine.PlaybackExpectation(play)
		if err != nil {
			return err
		}
		if err = client.Checkpoint.Track(eid, raw, round); err != nil {
			return err
		}
		r.Native.Outbound = append(r.Native.Outbound, NativePvpOutbound{Index: len(r.Native.Outbound) + 1, Target: id, Generation: client.Generation, Playback: *play})
		r.Native.Clients[id] = client
	}
	return nil
}

func rebuildNativeClient(r *HumanPvpRoom, id string) error {
	client := r.Native.Clients[id]
	started := client.Checkpoint.StartedSequence
	last := client.Checkpoint.LastSequence
	client.Checkpoint = nativeengine.Checkpoint{Generation: client.Generation, StartedSequence: started, LastSequence: last}
	client.CanonicalCursor = 0
	client.WindowHead = ""
	r.Native.Clients[id] = client
	return flushNativePlaybacks(r, id)
}

func appendNativeWindow(r *HumanPvpRoom) error {
	if !r.Native.Update.State.AwaitingPlayer || r.Native.Update.State.CurrentInputEID == "" {
		return nil
	}
	for _, id := range nativeRecipients(r) {
		client := r.Native.Clients[id]
		if client.CanonicalCursor != len(r.Native.Canonical) || client.WindowHead == r.Native.Journal.Head {
			continue
		}
		eid := client.Mapping[r.Native.Update.State.CurrentInputEID]
		if eid == "" {
			continue
		}
		play := nativeengine.Playback{Method: "revival_restore_input", Args: []any{eid, r.Native.Update.State.InputTime}}
		r.Native.Outbound = append(r.Native.Outbound, NativePvpOutbound{Index: len(r.Native.Outbound) + 1, Target: id, Generation: client.Generation, Playback: play, Window: true})
		client.WindowHead = r.Native.Journal.Head
		r.Native.Clients[id] = client
	}
	return nil
}

func (s *Service) nativeAdvance(ctx context.Context, r *HumanPvpRoom, request map[string]any) error {
	authority, err := s.nativeAuthority(r.UUID)
	if err != nil {
		return err
	}
	journal, raw, err := authority.Advance(ctx, &r.Native.Journal, request)
	if err != nil {
		return err
	}
	update, err := nativeengine.ParseUpdate(raw)
	if err != nil {
		return err
	}
	r.Native.Journal, r.Native.Update = *journal, *update
	r.Native.WindowDeadline = 0
	if err = s.appendNativeCanonical(r, update); err != nil {
		return err
	}
	if update.Result != nil {
		if len(update.Result.Winners) != 1 || update.Result.Winners[0] != r.IDs[0] && update.Result.Winners[0] != r.IDs[1] {
			return errors.New("PVP原生引擎没有产生房间内唯一胜方")
		}
		r.Native.Winners = append([]string(nil), update.Result.Winners...)
		if r.Native.ResultDeadline == 0 {
			r.Native.ResultDeadline = s.Now().Unix() + 120
		}
	}
	return nil
}

func (s *Service) advanceNativeUntilBoundary(ctx context.Context, r *HumanPvpRoom, first map[string]any) error {
	request := first
	for count := 0; count < 512; count++ {
		if err := s.nativeAdvance(ctx, r, request); err != nil {
			return err
		}
		if r.Native.Update.Result != nil {
			return nil
		}
		if r.Native.Update.State.AwaitingPlayer {
			return appendNativeWindow(r)
		}
		request = map[string]any{"operation": "drive"}
	}
	return errors.New("PVP原生推进超过单事务保护步数")
}

func nativeClientGeneration(envelope *battleEnvelope) int64 { return envelope.Generation }

func nativeClientEID(data map[string]any) string {
	eid, _ := data["eid"].(string)
	return eid
}

func nativeClientCommand(data map[string]any) any { return data["command"] }

func nativeClientWinners(data map[string]any) []string {
	values, _ := data["winner_eids"].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if id, ok := value.(string); ok {
			result = append(result, id)
		}
	}
	return result
}

func nativeDeadlineSeconds(update nativeengine.Update) int64 {
	seconds := int64(math.Ceil(update.State.InputTime))
	if seconds <= 0 || seconds > 20 {
		seconds = 20
	}
	return seconds
}

func (s *Service) tickNativePvp(ctx context.Context, c *Connection) {
	local := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if local == nil || local.Native == nil || local.Status != "fighting" {
		return
	}
	_, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, _ map[string]*Avatar, _ string) error {
		if r.Native == nil || r.Native.Update.Result != nil || !r.Native.Update.State.AwaitingPlayer || !nativeAllClientsReady(r) {
			return nil
		}
		now := s.Now().Unix()
		if r.Native.WindowDeadline == 0 {
			r.Native.WindowDeadline = now + nativeDeadlineSeconds(r.Native.Update)
			return nil
		}
		if now < r.Native.WindowDeadline {
			return nil
		}
		return s.advanceNativeUntilBoundary(ctx, r, map[string]any{"operation": "timeout"})
	})
	if err != nil {
		log.Printf("PVP单原生超时推进失败，将由已提交日志重试：%v", err)
	}
}

func nativeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

func (s *Service) nativeHumanDoCommand(ctx context.Context, c *Connection, command string, args []any) (bool, []Push, error) {
	if command != "move_to" && command != "use_skill" {
		return true, nil, errors.New("PVP单原生权威只接受移动与技能点击")
	}
	_, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, _ map[string]*Avatar, id string) error {
		if r.Native == nil || r.Status != "fighting" || r.Native.Update.Result != nil {
			return errors.New("PVP单原生房间未处于输入状态")
		}
		if !nativeAllClientsReady(r) {
			return errors.New("PVP双方尚未完成上一条权威命令播放")
		}
		client := r.Native.Clients[id]
		click, e := nativeengine.MapClick(command, args, r.Native.Update.State, [2]string{r.IDs[0], r.IDs[1]}, id, client.Mapping, client.Frame)
		if e != nil {
			return e
		}
		return s.advanceNativeUntilBoundary(ctx, r, map[string]any{"operation": "step", "command": click})
	})
	return true, nil, err
}

func nativeRecordBattleEvent(b *BattleSession, envelope *battleEnvelope) {
	b.EventLog = append(b.EventLog, BattleEvent{Sequence: envelope.Sequence, Kind: envelope.Kind, Data: cloneEventData(envelope.Data)})
	if len(b.EventLog) > battleEventLogMax {
		b.EventLog = b.EventLog[len(b.EventLog)-battleEventLogMax:]
	}
	for eventLogBytes(b.EventLog) > battleEventLogBytes && len(b.EventLog) > 1 {
		b.EventLog = b.EventLog[1:]
	}
}

func (s *Service) absorbNativeHumanBattleEvent(ctx context.Context, c *Connection, envelope *battleEnvelope) (bool, []Push, error) {
	settled, aborted := false, false
	r, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		if r.Native == nil {
			return errors.New("PVP单原生房间状态缺失")
		}
		b := v[id].Progress.Battle
		if b == nil || b.UUID != r.UUID {
			return errors.New("PVP单原生战斗归属失配")
		}
		client, ok := r.Native.Clients[id]
		if !ok {
			return errors.New("PVP客户端权威状态缺失")
		}
		if r.Status == "settled" {
			if envelope.Kind != "result" || nativeClientGeneration(envelope) != client.Generation || client.Checkpoint.ResultCandidate == nil || envelope.Sequence != client.Checkpoint.ResultCandidate.Sequence {
				return errors.New("PVP结算后只接受原末态重试")
			}
			if !nativeResultRetryMatches(b, envelope) {
				return errors.New("PVP结算重试正文被更改")
			}
			if !client.Checkpoint.TerminalMatches(r.Native.Winners, len(client.Checkpoint.Commands)) {
				return errors.New("PVP结算重试与权威末态不符")
			}
			return nil
		}
		if r.Status == "aborted" {
			return errors.New("PVP单原生房间已中断")
		}
		if envelope.Kind == "error" {
			e := s.nativeObservePlayback(ctx, r, b, id, envelope)
			if e == nil || e.Error() != "客户端PVP权威播放桥报错" {
				return e
			}
			humanAbort(r, v, "客户端PVP权威播放桥报错")
			aborted = true
			return nil
		}
		if e := s.nativeObservePlayback(ctx, r, b, id, envelope); e != nil {
			return e
		}
		kind := envelope.Kind
		if kind == "result" && len(r.Native.Winners) == 1 {
			ready := true
			for _, oid := range r.IDs {
				cp := r.Native.Clients[oid].Checkpoint
				ready = ready && cp.TerminalMatches(r.Native.Winners, len(cp.Commands))
			}
			if ready {
				r.Winner = r.Native.Winners[0]
				r.FinalDigest = r.Native.Journal.Head
				if err := settleHumanPvp(r, v, s.Now().Unix()); err != nil {
					return err
				}
				settled = true
			}
		}
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	if settled || aborted {
		s.closeNativeAuthority(r.UUID)
	}
	return true, nil, nil
}

// nativeObservePlayback供双端真人与单端PVP复用同一映射、FIFO与末态校验。
func (s *Service) nativeObservePlayback(ctx context.Context, r *HumanPvpRoom, b *BattleSession, id string, envelope *battleEnvelope) error {
	client := r.Native.Clients[id]
	if nativeClientGeneration(envelope) != client.Generation {
		return errors.New("PVP客户端事件恢复世代失配")
	}
	if envelope.Kind == "started" && (r.Status != "fighting" || !b.Started) {
		return errors.New("PVP尚未由共同加载屏障允许开战")
	}
	if err := acceptBattleEvent(b, envelope); err != nil {
		return err
	}
	nativeRecordBattleEvent(b, envelope)
	if envelope.Kind == "ready" {
		revision, ok := envelope.Data["bridge_revision"].(float64)
		if !ok || int(revision) != 31 {
			return errors.New("PVP单原生客户端桥版本不符")
		}
		return nil
	}
	if envelope.Kind == "settings" {
		if automatic, _ := envelope.Data["auto_battle"].(bool); automatic && len(nativeRecipients(r)) > 1 {
			return errors.New("PVP单原生房间禁止客户端自行开启自动")
		}
		return nil
	}
	if envelope.Kind == "error" {
		return errors.New("客户端PVP权威播放桥报错")
	}
	if envelope.Kind == "started" {
		units, e := nativeUnitSnapshots(envelope.Data["units"])
		if e != nil {
			return e
		}
		mapping := nativeengine.BindEntities(r.Native.InitialUnits, units, id == r.IDs[0], nil, nil)
		frame, e := nativeengine.DeriveCoordinateFrame(r.Native.InitialUnits, units, mapping)
		if e != nil {
			return e
		}
		mapping = nativeengine.BindEntities(r.Native.InitialUnits, units, id == r.IDs[0], mapping, frame)
		client.Mapping, client.Frame = mapping, frame
		if e = client.Checkpoint.Observe(client.Generation, envelope.Sequence, "started", "", nil, nil); e != nil {
			return e
		}
		r.Native.Clients[id] = client
		if e = rebuildNativeClient(r, id); e != nil {
			return e
		}
		if nativeAllClientsStarted(r) && !r.Native.Activated {
			r.Native.Activated = true
			if r.Native.Update.Result != nil {
				return nil
			}
			if r.Native.Update.State.AwaitingPlayer {
				return appendNativeWindow(r)
			}
			return s.advanceNativeUntilBoundary(ctx, r, map[string]any{"operation": "drive"})
		}
		if nativeAllClientsStarted(r) && r.Native.Activated && r.Native.Update.Result == nil && r.Native.Update.State.AwaitingPlayer {
			return appendNativeWindow(r)
		}
		return nil
	}
	if envelope.Kind == "snapshot" {
		if raw, exists := envelope.Data["units"]; exists {
			units, e := nativeUnitSnapshots(raw)
			if e != nil {
				return e
			}
			client.Mapping = nativeengine.BindEntities(r.Native.Update.State.Units, units, id == r.IDs[0], client.Mapping, client.Frame)
			r.Native.Clients[id] = client
			if e = flushNativePlaybacks(r, id); e != nil {
				return e
			}
			if r.Native.Update.State.AwaitingPlayer {
				return appendNativeWindow(r)
			}
		}
		return nil
	}
	kind := envelope.Kind
	if kind != "command" && kind != "turn" && kind != "input" && kind != "round_end" && kind != "result" {
		return nil
	}
	if kind == "result" && r.Native.Update.Result == nil {
		return errors.New("客户端不能先于PVP原生引擎提交胜方")
	}
	winners := []string(nil)
	if kind == "result" {
		winners = nativeClientWinners(envelope.Data)
	}
	if err := client.Checkpoint.Observe(client.Generation, envelope.Sequence, kind, nativeClientEID(envelope.Data), nativeClientCommand(envelope.Data), winners); err != nil {
		return err
	}
	r.Native.Clients[id] = client
	if r.Native.Update.State.AwaitingPlayer && nativeAllClientsReady(r) && r.Native.WindowDeadline == 0 {
		r.Native.WindowDeadline = s.Now().Unix() + nativeDeadlineSeconds(r.Native.Update)
	}
	return nil
}

func nativeRecipients(r *HumanPvpRoom) []string {
	if r.Native == nil {
		return nil
	}
	ids := []string{}
	for _, id := range r.IDs {
		if _, ok := r.Native.Clients[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
