package game

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hs-server/internal/mobileproto"
	"hs-server/internal/nativeengine"
	"log"
	"math"
	"sort"
	"time"
)

//go:embed human_battle_bridge_script.py
var humanBattleBridgeScript string

// 房间完整镜像在双方行锁存档内保存；UUID/seed/所有卡快照与输入日志只能同事务改变。
type HumanPvpRoom struct {
	UUID        string                    `json:"uuid"`
	IDs         []string                  `json:"ids"`
	Seed        int64                     `json:"seed"`
	Dungeon     int                       `json:"dungeon"`
	BattleID    int                       `json:"battle_id"`
	Rated       bool                      `json:"rated"`
	Status      string                    `json:"status"`
	Created     int64                     `json:"created"`
	Players     map[string]HumanPvpPlayer `json:"players"`
	Inputs      map[string]HumanPvpInput  `json:"inputs,omitempty"`
	Commands    []HumanPvpCommand         `json:"commands,omitempty"`
	Results     map[string]HumanPvpResult `json:"results,omitempty"`
	Recovering  map[string]int            `json:"recovering,omitempty"`
	Winner      string                    `json:"winner,omitempty"`
	FinalDigest string                    `json:"final_digest,omitempty"`
	Reason      string                    `json:"reason,omitempty"`
	Pending     *HumanPvpCommand          `json:"pending,omitempty"`
	LineupNum   int                       `json:"lineup_num"`
	Deliveries  []HumanDelivery           `json:"deliveries,omitempty"`
	Native      *NativePvpRoom            `json:"native,omitempty"`
}
type HumanPvpPlayer struct {
	Profile      SocialProfile           `json:"profile"`
	Cards        []Card                  `json:"cards"`
	Runes        map[string]Rune         `json:"runes"`
	PresetIDs    []string                `json:"preset_ids"`
	Presets      map[string]PresetRecord `json:"presets"`
	Layout       BattleLayout            `json:"layout"`
	Selected     bool                    `json:"selected"`
	Loaded       bool                    `json:"loaded"`
	Ready        bool                    `json:"ready"`
	Started      bool                    `json:"started"`
	Result       *SyncPvpResult          `json:"result,omitempty"`
	AfterHighest int                     `json:"after_highest,omitempty"`
	AfterStreak  int                     `json:"after_streak,omitempty"`
	AfterWins    int                     `json:"after_wins,omitempty"`
}
type HumanPvpInput struct {
	Action int64            `json:"action"`
	EID    string           `json:"eid"`
	Master string           `json:"master"`
	Digest string           `json:"digest"`
	Units  []map[string]any `json:"units"`
}
type HumanPvpCommand struct {
	Index       int    `json:"index"`
	Action      int64  `json:"action"`
	EID         string `json:"eid"`
	Master      string `json:"master"`
	Name        string `json:"name"`
	Args        []any  `json:"args"`
	InputDigest string `json:"input_digest"`
}
type HumanPvpResult struct {
	Winner       string `json:"winner"`
	Action       int64  `json:"action"`
	Digest       string `json:"digest"`
	Sequence     int64  `json:"sequence"`
	CommandIndex int    `json:"command_index"`
}

func cloneHumanRoom(r *HumanPvpRoom) *HumanPvpRoom {
	raw, _ := json.Marshal(r)
	var out HumanPvpRoom
	_ = json.Unmarshal(raw, &out)
	if out.Inputs == nil {
		out.Inputs = map[string]HumanPvpInput{}
	}
	if out.Results == nil {
		out.Results = map[string]HumanPvpResult{}
	}
	if out.Recovering == nil {
		out.Recovering = map[string]int{}
	}
	return &out
}
func humanRoomActive(r *HumanPvpRoom) bool {
	return r != nil && r.Status != "settled" && r.Status != "aborted"
}
func humanOwnCards(p HumanPvpPlayer) map[string]any {
	return cardMgrPropertiesWithRunes(p.Cards, p.Runes)
}
func humanSelection(r *HumanPvpRoom) Push {
	infos := mobileproto.Map{}
	for _, id := range r.IDs {
		p := r.Players[id]
		presetIDs := []any{}
		records := mobileproto.Map{}
		for _, pid := range p.PresetIDs {
			presetIDs = append(presetIDs, ObjectID(pid))
			rec := p.Presets[pid]
			records = append(records, mobileproto.Pair{Key: ObjectID(pid), Value: map[string]any{"fighting_cards": battleSlotWire(rec.Cards.Fighting), "support_cards": battleSlotWire(rec.Cards.Support)}})
		}
		cards := []any{}
		mgr := humanOwnCards(p)
		for _, card := range p.Cards {
			wire := mgr[card.UUID].(map[string]any)
			wire["uuid"] = ObjectID(card.UUID)
			cards = append(cards, wire)
		}
		infos = append(infos, mobileproto.Pair{Key: ObjectID(id), Value: map[string]any{"avatar_info": p.Profile.wire(), "preset_ids": presetIDs, "cards_record": records, "cards": cards}})
	}
	return push("Avatar", "select_pvp_cards", infos, r.LineupNum)
}
func (s *Service) newHumanRoom(v map[string]*Avatar, ids []string, rated bool) (*HumanPvpRoom, error) {
	sort.Strings(ids)
	var seed [8]byte
	if _, e := rand.Read(seed[:]); e != nil {
		return nil, e
	}
	r := &HumanPvpRoom{UUID: newBattleUUID(s.Now()), IDs: append([]string{}, ids...), Seed: int64(binary.LittleEndian.Uint64(seed[:]) & 0x7fffffff), Dungeon: socialCatalog.Constants["CHALLENGE_DUNGEON_ID"], Rated: rated, Status: "select", Created: s.Now().Unix(), Players: map[string]HumanPvpPlayer{}, Inputs: map[string]HumanPvpInput{}, Results: map[string]HumanPvpResult{}, Recovering: map[string]int{}, LineupNum: 1}
	if rated {
		r.Dungeon = syncPvpRule().DungeonID
		rule, e := syncPvpScoreRuleFor(v[ids[0]].Progress.SyncPvpScore)
		if e != nil {
			return nil, e
		}
		r.BattleID = rule.DungeonBattleID
		r.LineupNum = rule.LineupNum
	} else {
		r.BattleID = dungeonCatalog[r.Dungeon].BattleID
	}
	if r.BattleID <= 0 || r.Dungeon <= 0 {
		return nil, errors.New("真人竞技副本目录缺失")
	}
	for _, id := range ids {
		av := v[id]
		p := &av.Progress
		if p.Battle != nil && !p.Battle.Finished || humanRoomActive(p.Social.HumanRoom) {
			return nil, socialFailure("RET_CHALLENGE_BUSY", "对方正处于战斗")
		}
		if e := ensureSyncPvpPeriod(p, s.Now()); e != nil {
			return nil, e
		}
		ensurePresetState(p, s.Now())
		own := HumanPvpPlayer{Profile: socialProfile(*av), Cards: append([]Card{}, p.Cards...), Runes: CloneProgress(*p).Runes, PresetIDs: append([]string{}, p.SyncPvpPresetIDs...), Presets: map[string]PresetRecord{}}
		for _, pid := range own.PresetIDs {
			own.Presets[pid] = p.PresetCardsRecord[pid]
		}
		if len(own.PresetIDs) == 0 {
			pid := newBattleUUID(s.Now())
			layout := savedBattleLayout(*p, syncPvpRule().DungeonID)
			if len(layout.team()) == 0 && len(p.Cards) > 0 {
				layout = BattleLayout{Fighting: []string{p.Cards[0].UUID}}
			}
			if e := validatePresetCards(*p, layout, false); e != nil {
				return nil, e
			}
			own.PresetIDs = []string{pid}
			own.Presets[pid] = PresetRecord{PresetID: pid, Cards: layout}
		}
		r.Players[id] = own
	}
	for _, id := range ids {
		v[id].Progress.Social.HumanRoom = cloneHumanRoom(r)
		v[id].Progress.SyncPvpMatch = nil
	}
	for _, id := range ids {
		appendHumanDelivery(r, id, "select", 0, false)
	}
	for _, id := range ids {
		v[id].Progress.Social.HumanRoom = cloneHumanRoom(r)
	}
	return r, nil
}

// humanTransaction验证双方房间镜像相等后原子变更，任何事务失败不投递动作。
func (s *Service) humanTransaction(ctx context.Context, c *Connection, fn func(*HumanPvpRoom, map[string]*Avatar, string) error) (*HumanPvpRoom, error) {
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持真人房间")
	}
	ownID := hexOf(selectedOID(c))
	avatars, e := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{ownID}, Limit: 1})
	if e != nil {
		return nil, e
	}
	if len(avatars) != 1 || avatars[0].Progress.Social.HumanRoom == nil {
		return nil, errors.New("没有真人房间")
	}
	initial := avatars[0].Progress.Social.HumanRoom
	var result *HumanPvpRoom
	rows, e := store.UpdateSocial(ctx, initial.IDs, func(v map[string]*Avatar) error {
		raw := []byte{}
		for _, id := range initial.IDs {
			av := v[id]
			if av == nil || av.Progress.Social.HumanRoom == nil || av.Progress.Social.HumanRoom.UUID != initial.UUID {
				return errors.New("真人双方房间失配")
			}
			blob, _ := json.Marshal(av.Progress.Social.HumanRoom)
			if raw == nil || len(raw) == 0 {
				raw = blob
			} else if !bytes.Equal(raw, blob) {
				return errors.New("真人双方房间镜像不一致")
			}
		}
		result = cloneHumanRoom(v[initial.IDs[0]].Progress.Social.HumanRoom)
		before := cloneHumanRoom(result)
		if e := fn(result, v, ownID); e != nil {
			return e
		}
		if e := recordHumanTransitions(before, result); e != nil {
			return e
		}
		for _, id := range result.IDs {
			v[id].Progress.Social.HumanRoom = cloneHumanRoom(result)
			v[id].Progress.Social.Revision++
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	acceptSocialRows(c, rows)
	return result, nil
}
func (s *Service) humanChallengeRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	cb, ok := callbackArg(args)
	var peerID string
	need := 2
	if method == "refuse_challenge" {
		need = 3
	}
	if c.phase != Playing || !ok || len(args) != need || json.Unmarshal(args[1], &peerID) != nil {
		return nil, errors.New("真人好友邀请参数无效")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持真人邀请")
	}
	ownID := hexOf(selectedOID(c))
	onlineIDs, e := s.humanOnlineIDs(ctx)
	if e != nil {
		return nil, e
	}
	online := onlineIDs[peerID]
	var room *HumanPvpRoom
	reason := 0
	notifyRefuse := false
	if method == "refuse_challenge" {
		if json.Unmarshal(args[2], &reason) != nil || (reason != 0 && reason != 1) {
			return nil, errors.New("拒绝邀请原因无效")
		}
	}
	rows, e := store.UpdateSocial(ctx, []string{ownID, peerID}, func(v map[string]*Avatar) error {
		own, peer := v[ownID], v[peerID]
		if own == nil || peer == nil {
			return socialFailure("RET_CHALLENGE_NOT_FRIEND", "好友不存在")
		}
		ensureSocial(&own.Progress.Social)
		ensureSocial(&peer.Progress.Social)
		if e := AuthorizedFriendRelationship(*own, *peer); e != nil {
			return socialFailure("RET_CHALLENGE_NOT_FRIEND", e.Error())
		}
		switch method {
		case "challenge_friend":
			if !online {
				return socialFailure("RET_CHALLENGE_FRIEND_OFFLINE", "好友已离线")
			}
			if own.Progress.Battle != nil && !own.Progress.Battle.Finished || peer.Progress.Battle != nil && !peer.Progress.Battle.Finished || humanRoomActive(own.Progress.Social.HumanRoom) || humanRoomActive(peer.Progress.Social.HumanRoom) {
				return socialFailure("RET_CHALLENGE_BUSY", "玩家正在战斗")
			}
			if len(peer.Progress.Social.Challenges) >= socialCatalog.Constants["MAX_CHALLENGE_COUNT"] {
				return socialFailure("RET_CHALLENGE_IN_CHALLENGE", "邀请队列已满")
			}
			if _, exists := peer.Progress.Social.Challenges[ownID]; !exists {
				peer.Progress.Social.Challenges[ownID] = SocialFriend{Info: socialProfile(*own), Time: s.Now().Unix()}
				own.Progress.Social.ChallengeSent[peerID] = s.Now().Unix()
			}
		case "agree_challenge":
			invite, exists := own.Progress.Social.Challenges[peerID]
			if !exists || s.Now().Unix() < invite.Time || s.Now().Unix()-invite.Time > int64(socialCatalog.Constants["CHALLENGE_WAIT_TIME"]) {
				return socialFailure("RET_FAILED", "邀请已失效")
			}
			var e error
			room, e = s.newHumanRoom(v, []string{ownID, peerID}, false)
			if e != nil {
				return e
			}
			delete(own.Progress.Social.Challenges, peerID)
			delete(peer.Progress.Social.ChallengeSent, ownID)
		case "cancel_challenge":
			delete(peer.Progress.Social.Challenges, ownID)
			delete(own.Progress.Social.ChallengeSent, peerID)
		case "refuse_challenge":
			delete(own.Progress.Social.Challenges, peerID)
			delete(peer.Progress.Social.ChallengeSent, ownID)
			notifyRefuse = true
			sequence := int64(1)
			if n := len(peer.Progress.Social.HumanRefusals); n > 0 {
				sequence = peer.Progress.Social.HumanRefusals[n-1].Sequence + 1
			}
			peer.Progress.Social.HumanRefusals = append(peer.Progress.Social.HumanRefusals, HumanRefusal{Sequence: sequence, Profile: socialProfile(*own), Reason: reason, Created: s.Now().Unix()})
			if n := len(peer.Progress.Social.HumanRefusals); n > 32 {
				peer.Progress.Social.HumanRefusals = peer.Progress.Social.HumanRefusals[n-32:]
			}
		default:
			return errors.New("未知真人好友邀请方法")
		}
		own.Progress.Social.Revision++
		peer.Progress.Social.Revision++
		return nil
	})
	if e != nil {
		var se socialError
		if errors.As(e, &se) {
			return []Push{Callback(cb, []any{se.code})}, nil
		}
		return nil, e
	}
	acceptSocialRows(c, rows)
	_ = room
	_ = notifyRefuse
	return append(s.socialLivePushes(c.SelectedAvatarUnsafe().Progress.Social), Callback(cb, []any{RetSuccess})), nil
}
func humanLoadingPushes(r *HumanPvpRoom, id string) []Push {
	own := r.Players[id]
	players := []any{}
	teams := mobileproto.Map{}
	for _, oid := range r.IDs {
		players = append(players, ObjectID(oid))
		teams = append(teams, mobileproto.Pair{Key: ObjectID(oid), Value: map[string]any{"fighting_cards": battleSlotWire(r.Players[oid].Layout.Fighting), "support_cards": battleSlotWire(r.Players[oid].Layout.Support)}})
	}
	typ := socialCatalog.Constants["CHALLENGE_BATTLE"]
	if r.Rated {
		typ = socialCatalog.Constants["SYNC_PVP_BATTLE"]
	}
	extra := map[string]any{"human_shared": true, "sync_pvp": r.Rated}
	if r.Native != nil {
		extra["server_authoritative_pvp"] = true
		extra["server_authority_generation"] = r.Native.Clients[id].Generation
	}
	return []Push{push("Avatar", "on_query_hotfix_success", clientExtensionsScript(), bridgeHotfixIndex), push("Avatar", "start_server_battle_ok", typ, r.Dungeon, ObjectID(r.UUID), extra), push("Avatar", "pvp_battle_start_load", teams), battleSync("set_last_fighting_cards", lastFightingWire(ObjectID(id), own.Layout.Fighting, own.Layout.Support)), battleSync("prepare", players, r.BattleID, r.Seed, r.Dungeon), battleSync("revival_set_battle_preferences", 1, false)}
}
func humanStartPushes(r *HumanPvpRoom) []Push {
	result := []Push{}
	for _, id := range r.IDs {
		player := r.Players[id]
		cards := []map[string]any{}
		mgr := humanOwnCards(player)
		for _, uuid := range player.Layout.team() {
			cards = append(cards, mgr[uuid].(map[string]any))
		}
		result = append(result, battleSync("add_fighting_cards", battleSlotWire(player.Layout.team()), pvpCardList(cards), ObjectID(id)))
	}
	return append(result, battleSync("battle_fighting"), battleSync("start"), battleSync("check_on_battle_start"))
}
func (s *Service) humanCardsRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) (bool, []Push, error) {
	if !humanRoomActive(c.SelectedAvatarUnsafe().Progress.Social.HumanRoom) {
		return false, nil, nil
	}
	if method == "send_pvp_cards" {
		index := 0
		if len(args) == 2 {
			if _, ok := callbackArg(args); !ok {
				return true, nil, errors.New("真人选卡回调无效")
			}
			index = 1
		}
		if len(args) != index+1 {
			return true, nil, errors.New("真人选卡参数无效")
		}
		var presetIndex int
		layout, e := parseBattleLayout(args[index])
		if json.Unmarshal(args[index], &presetIndex) == nil {
			player := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom.Players[hexOf(selectedOID(c))]
			if presetIndex < 0 || presetIndex >= len(player.PresetIDs) {
				return true, nil, errors.New("真人预设索引无效")
			}
			layout = cloneBattleLayout(player.Presets[player.PresetIDs[presetIndex]].Cards)
			e = nil
		}
		if e != nil {
			return true, nil, e
		}
		loadedNow := false
		r, e := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
			if r.Status != "select" {
				previous := r.Players[id].Layout
				if r.Players[id].Selected && sameBattleSlots(previous.Fighting, layout.Fighting) && sameBattleSlots(previous.Support, layout.Support) {
					return nil
				}
				return errors.New("真人房间已经锁定选卡")
			}
			p := &v[id].Progress
			if e := validatePresetCards(*p, layout, false); e != nil {
				return e
			}
			player := r.Players[id]
			player.Layout = cloneBattleLayout(layout)
			player.Selected = true
			r.Players[id] = player
			all := true
			for _, oid := range r.IDs {
				all = all && r.Players[oid].Selected
			}
			if all {
				r.Status = "loading"
				loadedNow = true
				for _, oid := range r.IDs {
					own := r.Players[oid]
					v[oid].Progress.Battle = &BattleSession{UUID: r.UUID, DungeonID: r.Dungeon, BattleID: r.BattleID, Seed: r.Seed, Status: "准备", CreatedAt: s.Now().Unix(), Team: own.Layout.team(), Layout: &own.Layout, Extra: map[string]any{"human_shared": true, "sync_pvp": r.Rated}}
				}
				if s.nativePvpConfigured() {
					if e := s.beginNativePvp(ctx, r); e != nil {
						return e
					}
				}
			}
			return nil
		})
		if e != nil {
			return true, nil, e
		}
		_ = loadedNow
		_ = r
		if index == 1 {
			cb, _ := callbackArg(args)
			return true, []Push{Callback(cb, []any{RetSuccess})}, nil
		}
		return true, []Push{push("Avatar", "set_cards_result", true, "")}, nil
	}
	if method == "pvp_load_complete" {
		if len(args) > 1 {
			return true, nil, errors.New("真人加载参数无效")
		}
		if len(args) == 1 {
			if _, ok := callbackArg(args); !ok {
				return true, nil, errors.New("真人加载回调无效")
			}
		}
		r, e := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
			if (r.Status != "loading" && r.Status != "fighting") || v[id].Progress.Battle == nil {
				return errors.New("真人房间尚未建立战斗")
			}
			player := r.Players[id]
			player.Loaded = true
			r.Players[id] = player
			v[id].Progress.Battle.Loaded = true
			return nil
		})
		_ = r
		if e != nil {
			return true, nil, e
		}
		if len(args) == 1 {
			cb, ok := callbackArg(args)
			if !ok {
				return true, nil, errors.New("真人加载回调无效")
			}
			return true, []Push{Callback(cb, []any{RetSuccess})}, nil
		}
		return true, nil, nil
	}
	return false, nil, nil
}

func humanDigest(units any) (string, []map[string]any, error) {
	raw, e := json.Marshal(units)
	if e != nil {
		return "", nil, e
	}
	var rows []map[string]any
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 || len(rows) > battleRosterMax {
		return "", nil, errors.New("真人快照无效")
	}
	var normalized []any
	if json.Unmarshal(raw, &normalized) != nil {
		return "", nil, errors.New("真人单位快照编码无效")
	}
	if e := validateBattleSnapshot(map[string]any{"units": normalized}); e != nil {
		return "", nil, e
	}
	seen := map[string]bool{}
	for _, row := range rows {
		id, ok := row["eid"].(string)
		if !ok || id == "" || seen[id] {
			return "", nil, errors.New("真人单位标识缺失或重复")
		}
		seen[id] = true
		for _, key := range []string{"hp", "max_hp", "ap"} {
			if number, exists := row[key]; exists {
				n, ok := number.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return "", nil, errors.New("真人单位数值无效")
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["eid"].(string) < rows[j]["eid"].(string) })
	raw, _ = json.Marshal(rows)
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("%x", digest), rows, nil
}
func humanInput(data map[string]any) (HumanPvpInput, error) {
	var v HumanPvpInput
	action, ok := data["action"].(float64)
	if !ok || action < 0 || action != float64(int64(action)) {
		return v, errors.New("真人行动序号无效")
	}
	v.Action = int64(action)
	v.EID, _ = data["eid"].(string)
	v.Master, _ = data["master"].(string)
	digest, units, e := humanDigest(data["units"])
	if e != nil {
		return v, e
	}
	v.Digest, v.Units = digest, units
	if v.EID == "" || !validObjectID(v.Master) {
		return v, errors.New("真人输入控制权无效")
	}
	return v, nil
}
func humanInputsAgree(r *HumanPvpRoom) (HumanPvpInput, bool) {
	a, aok := r.Inputs[r.IDs[0]]
	b, bok := r.Inputs[r.IDs[1]]
	return a, aok && bok && a.Action == b.Action && a.EID == b.EID && a.Master == b.Master && a.Digest == b.Digest
}
func humanCommandPush(r *HumanPvpRoom, cmd HumanPvpCommand) Push {
	return battleSync("revival_do_shared_command", ObjectID(cmd.Master), cmd.Name, cmd.Args, cmd.Index, r.UUID)
}

// 真人命令仅在双方已到同一行动/输入单位/控制者/状态屏障时广播，一次提交同时投递两端。
func (s *Service) humanDoCommand(ctx context.Context, c *Connection, command string, args []any) (bool, []Push, error) {
	local := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if !humanRoomActive(local) {
		return false, nil, nil
	}
	if local.Native != nil {
		return s.nativeHumanDoCommand(ctx, c, command, args)
	}
	if command != "move_to" && command != "use_skill" {
		return true, nil, errors.New("真人房间只接受已取证原生命令")
	}
	var submitted HumanPvpCommand
	r, e := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		if r.Status != "fighting" {
			return errors.New("真人房间未共同开战")
		}
		frame, agree := humanInputsAgree(r)
		if !agree {
			var exists bool
			frame, exists = r.Inputs[id]
			if !exists {
				return errors.New("真人当前端尚未到输入屏障")
			}
		}
		if frame.Master != id {
			return errors.New("真人输入不属于当前鉴权玩家")
		}
		if len(r.Recovering) > 0 {
			return errors.New("对方正在恢复共享对局")
		}
		entityIndex := 0
		if command == "use_skill" && len(args) > 0 {
			if marker, ok := args[0].(string); ok && (marker == skillUsedClickMonster || marker == skillUsedClickSkill || marker == skillUsedClickHead) {
				entityIndex = 1
			}
		}
		if len(args) <= entityIndex {
			return errors.New("真人命令缺少单位")
		}
		entity, _ := args[entityIndex].(string)
		owned := false
		for _, u := range frame.Units {
			if u["eid"] == entity && u["master"] == id {
				owned = true
			}
		}
		if !owned {
			return errors.New("真人命令单位不受当前玩家控制")
		}
		if command == "move_to" && entity != frame.EID {
			return errors.New("真人移动不是当前输入单位")
		}
		if len(r.Commands) >= 2048 {
			return errors.New("真人输入日志达到服务端保护上限")
		}
		// 一个输入快照最多消费一次；相同网络重试不能重复执行。
		if len(r.Commands) > 0 {
			last := r.Commands[len(r.Commands)-1]
			if last.Action == frame.Action && last.InputDigest == frame.Digest {
				if last.Master == id && last.Name == command && bytes.Equal(mustJSON(last.Args), mustJSON(args)) {
					submitted = last
					return nil
				}
				return errors.New("真人输入屏障已消费")
			}
		}
		submitted = HumanPvpCommand{Index: len(r.Commands) + 1, Action: frame.Action, EID: entity, Master: id, Name: command, Args: args, InputDigest: frame.Digest}
		if !agree {
			if r.Pending != nil && !bytes.Equal(mustJSON(r.Pending), mustJSON(&submitted)) {
				return errors.New("真人输入已有待确认命令")
			}
			pending := submitted
			r.Pending = &pending
			submitted = HumanPvpCommand{}
			return nil
		}
		r.Pending = nil
		r.Commands = append(r.Commands, submitted)
		return nil
	})
	if e != nil {
		return true, nil, e
	}
	_ = r
	return true, nil, nil
}

// 双方加载且阵容与冻结预设逐槽相等后才共同开始；恢复端沿原日志重放。
func (s *Service) humanBattleFighting(ctx context.Context, c *Connection, args []json.RawMessage) (bool, []Push, error) {
	if !humanRoomActive(c.SelectedAvatarUnsafe().Progress.Social.HumanRoom) {
		return false, nil, nil
	}
	if c.phase != Playing || len(args) != 1 {
		return true, nil, errors.New("真人开战需要已鉴权玩家及阵容")
	}
	layout, err := parseBattleLayout(args[0])
	if err != nil {
		return true, nil, err
	}
	start := []string{}
	r, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		player := r.Players[id]
		if !player.Selected || !player.Loaded || v[id].Progress.Battle == nil {
			return errors.New("真人阵容或实体尚未加载")
		}
		if !sameBattleSlots(layout.Fighting, player.Layout.Fighting) || !sameBattleSlots(layout.Support, player.Layout.Support) {
			return errors.New("真人开战阵容与冻结快照不符")
		}
		if player.Started {
			return nil
		}
		player.Ready = true
		r.Players[id] = player
		if r.Status == "fighting" {
			start = append(start, id)
		} else {
			if r.Status != "loading" {
				return errors.New("真人房间不能开始")
			}
			for _, oid := range r.IDs {
				if !r.Players[oid].Ready {
					return nil
				}
			}
			r.Status = "fighting"
			start = append(start, r.IDs...)
		}
		for _, oid := range start {
			own := r.Players[oid]
			own.Started = true
			r.Players[oid] = own
			b := v[oid].Progress.Battle
			b.Started = true
			b.Status = "进行"
			b.AutoBattle = false
			b.BattleSpeed = 1
		}
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	_ = r
	c.battleStartSent = true
	return true, nil, nil
}
func humanReplayPush(r *HumanPvpRoom) Push {
	var commands []any
	_ = json.Unmarshal(mustJSON(r.Commands), &commands)
	return battleSync("revival_set_shared_replay", commands, r.UUID)
}

func validateHumanFrame(r *HumanPvpRoom, frame HumanPvpInput) error {
	found := false
	for _, row := range frame.Units {
		master, _ := row["master"].(string)
		card, _ := row["card_uuid"].(string)
		if master != "" {
			player, exists := r.Players[master]
			if !exists {
				return errors.New("真人快照单位控制者不是房间成员")
			}
			if card != "" {
				owned := false
				for _, uuid := range player.Layout.team() {
					owned = owned || uuid == card
				}
				if !owned {
					return errors.New("真人单位不是冻结出战卡")
				}
			}
		}
		if row["eid"] == frame.EID {
			if master != frame.Master {
				return errors.New("真人输入单位控制者不符")
			}
			found = true
		}
	}
	if !found {
		return errors.New("真人输入单位不在完整快照内")
	}
	return nil
}

func humanAbort(r *HumanPvpRoom, v map[string]*Avatar, reason string) {
	r.Status = "aborted"
	r.Reason = reason
	r.Pending = nil
	for _, id := range r.IDs {
		b := v[id].Progress.Battle
		if b != nil && b.UUID == r.UUID {
			b.Finished = true
			b.Status = "中断"
			b.Outcome = ""
			b.ResultCounted = true
			b.RewardGranted = true
		}
	}
}
func humanAbortPushes() []Push {
	return []Push{battleSync("revival_abort_shared"), push("Avatar", "exit_battle_ok")}
}

// 所有共享事件推进双方房间镜像；结果必须两端同胜方、同末态、同行动号。
func (s *Service) absorbHumanBattleEvent(ctx context.Context, c *Connection, envelope *battleEnvelope) (bool, []Push, error) {
	local := c.SelectedAvatarUnsafe().Progress.Social.HumanRoom
	if local == nil || envelope.BattleUUID != local.UUID {
		return false, nil, nil
	}
	if local.Native != nil {
		return s.absorbNativeHumanBattleEvent(ctx, c, envelope)
	}
	if envelope.Kind == "human_result" {
		if err := s.refreshPvpAwards(ctx, c); err != nil {
			return true, nil, err
		}
	}
	var broadcast *HumanPvpCommand
	settled, aborted := false, false
	r, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		b := v[id].Progress.Battle
		if b == nil || b.UUID != r.UUID {
			return errors.New("真人战斗归属失配")
		}
		if r.Status == "aborted" {
			return errors.New("真人房间已中断")
		}
		if r.Status == "settled" {
			previous, exists := r.Results[id]
			if envelope.Kind != "human_result" || !exists || envelope.Sequence != previous.Sequence {
				return errors.New("真人结算后拒绝新增事件")
			}
			result, e := humanResult(envelope)
			if e != nil {
				return e
			}
			if !bytes.Equal(mustJSON(result), mustJSON(previous)) {
				return errors.New("真人结果重试被篡改")
			}
			settled = true
			return nil
		}
		copy := *envelope
		if envelope.Kind == "started" && (r.Status != "fighting" || !b.Started) {
			return errors.New("真人尚未由共同加载屏障允许开战")
		}
		if envelope.Kind == "human_input" {
			copy.Kind = "input"
		}
		if envelope.Kind == "human_result" {
			copy.Kind = "result"
		}
		if e := acceptBattleEvent(b, &copy); e != nil {
			return e
		}
		b.EventLog = append(b.EventLog, BattleEvent{Sequence: envelope.Sequence, Kind: envelope.Kind, Data: cloneEventData(envelope.Data)})
		if len(b.EventLog) > battleEventLogMax {
			b.EventLog = b.EventLog[len(b.EventLog)-battleEventLogMax:]
		}
		for eventLogBytes(b.EventLog) > battleEventLogBytes && len(b.EventLog) > 1 {
			b.EventLog = b.EventLog[1:]
		}
		switch envelope.Kind {
		case "settings":
			if auto, _ := envelope.Data["auto_battle"].(bool); auto {
				return errors.New("真人共享战斗禁止单端自动输入")
			}
		case "result":
			return errors.New("真人结果必须使用共享结果信封")
		case "error":
			humanAbort(r, v, "客户端原生演算报错")
			aborted = true
		case "human_input":
			frame, e := humanInput(envelope.Data)
			if e != nil {
				return e
			}
			if e = validateHumanFrame(r, frame); e != nil {
				return e
			}
			index, e := humanCommandIndex(envelope.Data)
			if e != nil {
				return e
			}
			if target, recovering := r.Recovering[id]; recovering {
				if index < target {
					return nil
				}
				if index != target {
					return errors.New("真人恢复日志位置失配")
				}
				delete(r.Recovering, id)
			} else if index != len(r.Commands) {
				return errors.New("真人客户端执行命令位置失配")
			}
			if old, exists := r.Inputs[id]; exists && frame.Action < old.Action {
				return errors.New("真人行动序号回退")
			}
			r.Inputs[id] = frame
			other := r.IDs[0]
			if other == id {
				other = r.IDs[1]
			}
			if peer, exists := r.Inputs[other]; exists && peer.Action == frame.Action && len(r.Recovering) == 0 {
				matched, agree := humanInputsAgree(r)
				if !agree {
					humanAbort(r, v, "双方同一输入行动快照分歧")
					aborted = true
					return nil
				}
				if r.Pending != nil && r.Pending.Action == matched.Action {
					pending := *r.Pending
					if pending.InputDigest != matched.Digest || pending.Master != matched.Master {
						return errors.New("真人待确认输入与共同屏障不符")
					}
					r.Commands = append(r.Commands, pending)
					r.Pending = nil
					broadcast = &pending
				}
			}
		case "human_result":
			if r.Status != "fighting" {
				return errors.New("真人尚未共同开始")
			}
			result, e := humanResult(envelope)
			if e != nil {
				return e
			}
			if result.Winner != r.IDs[0] && result.Winner != r.IDs[1] {
				return errors.New("真人胜方不属于房间")
			}
			if result.CommandIndex != len(r.Commands) {
				return errors.New("真人结果缺少完整共享输入日志")
			}
			if old, exists := r.Results[id]; exists && !bytes.Equal(mustJSON(old), mustJSON(result)) {
				return errors.New("真人已提交结果不能覆盖")
			}
			delete(r.Recovering, id)
			r.Results[id] = result
			if len(r.Results) < 2 {
				return nil
			}
			a, z := r.Results[r.IDs[0]], r.Results[r.IDs[1]]
			if a.Winner != z.Winner || a.Action != z.Action || a.Digest != z.Digest || a.CommandIndex != z.CommandIndex {
				humanAbort(r, v, "双方最终胜负或末态分歧")
				aborted = true
				return nil
			}
			r.Winner = a.Winner
			r.FinalDigest = a.Digest
			if e = settleHumanPvp(r, v, s.Now().Unix()); e != nil {
				return e
			}
			settled = true
		}
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	_ = broadcast
	_ = aborted
	_ = settled
	_ = r
	return true, nil, nil
}
func humanCommandIndex(data map[string]any) (int, error) {
	n, ok := data["command_index"].(float64)
	if !ok || n < 0 || n > 2048 || n != float64(int(n)) {
		return 0, errors.New("真人命令日志位置无效")
	}
	return int(n), nil
}
func humanResult(envelope *battleEnvelope) (HumanPvpResult, error) {
	var result HumanPvpResult
	winners, err := battleWinnerEIDs(envelope.Data)
	if err != nil || len(winners) != 1 {
		return result, errors.New("真人必须明确唯一胜方")
	}
	frame, err := humanInput(map[string]any{"action": envelope.Data["action"], "eid": "结果", "master": winners[0], "units": envelope.Data["units"]})
	if err != nil {
		return result, err
	}
	index, err := humanCommandIndex(envelope.Data)
	if err != nil {
		return result, err
	}
	return HumanPvpResult{Winner: winners[0], Action: frame.Action, Digest: frame.Digest, Sequence: envelope.Sequence, CommandIndex: index}, nil
}

func humanFrozenCards(p HumanPvpPlayer) []map[string]any {
	result := []map[string]any{}
	mgr := humanOwnCards(p)
	for _, id := range p.Layout.team() {
		result = append(result, mgr[id].(map[string]any))
	}
	return result
}
func settleHumanPvp(r *HumanPvpRoom, v map[string]*Avatar, now int64) error {
	if r.Status == "settled" {
		return nil
	}
	for _, id := range r.IDs {
		p := &v[id].Progress
		own := r.Players[id]
		peerID := r.IDs[0]
		if peerID == id {
			peerID = r.IDs[1]
		}
		peer := r.Players[peerID]
		b := p.Battle
		b.Finished = true
		b.Outcome = "loss"
		if id == r.Winner {
			b.Outcome = "win"
		}
		b.Status = "结束"
		b.WinnerEIDs = []string{r.Winner}
		b.ResultCounted = true
		b.RewardGranted = true
		if r.Rated {
			if p.SyncPvpScore != own.Profile.Score {
				return errors.New("真人锁定积分被其它业务改变")
			}
			rule, err := syncPvpScoreRuleFor(own.Profile.Score)
			if err != nil {
				return err
			}
			exponent := math.Max(-100, math.Min(100, float64(peer.Profile.Score-own.Profile.Score)/400))
			expected := 1 / (1 + math.Pow(10, exponent))
			value := 0.0
			if id == r.Winner {
				value = 1
			}
			change := float64(rule.K) * (value - expected)
			if id != r.Winner {
				change *= rule.LoseProtectRatio
			}
			delta := syncPvpRound(change)
			if id == r.Winner && delta < 1 {
				delta = 1
			}
			old := p.SyncPvpScore
			p.SyncPvpScore = maxInt(syncPvpRule().InitScore, old+delta)
			p.SyncPvpHighestScore = maxInt(p.SyncPvpHighestScore, p.SyncPvpScore)
			next, err := syncPvpScoreRuleFor(p.SyncPvpScore)
			if err != nil {
				return err
			}
			if id == r.Winner {
				recordAchievementCompetitiveWin(p, 5, time.Unix(now, 0))
				p.SyncPvpWinStreak++
				p.SyncPvpWeeklyWins++
				p.SyncPvpLoseTimes = 0
			} else {
				p.SyncPvpWinStreak = 0
				p.SyncPvpLoseTimes++
			}
			result := SyncPvpResult{DeltaScore: p.SyncPvpScore - old, DivisionUpdated: next.ID - rule.ID, OwnScore: p.SyncPvpScore, EnemyScore: peer.Profile.Score}
			own.Result = &result
			own.AfterHighest = p.SyncPvpHighestScore
			own.AfterStreak = p.SyncPvpWinStreak
			own.AfterWins = p.SyncPvpWeeklyWins
			r.Players[id] = own
			if p.SyncPvpSettlements == nil {
				p.SyncPvpSettlements = map[string]SyncPvpReceipt{}
			}
			p.SyncPvpSettlements[r.UUID] = SyncPvpReceipt{Outcome: b.Outcome, Result: result}
			p.SyncPvpRecords = append(p.SyncPvpRecords, SyncPvpRecord{BattleUUID: r.UUID, Outcome: b.Outcome, Score: p.SyncPvpScore, CreatedAt: now, OwnInfo: own.Profile, EnemyInfo: peer.Profile, OwnTeam: own.Layout.team(), EnemyTeam: peer.Layout.team(), OwnCards: humanFrozenCards(own), EnemyCards: humanFrozenCards(peer)})
			if len(p.SyncPvpRecords) > syncPvpRule().RecordNum {
				p.SyncPvpRecords = p.SyncPvpRecords[len(p.SyncPvpRecords)-syncPvpRule().RecordNum:]
			}
		}
	}
	r.Status = "settled"
	r.Pending = nil
	return nil
}
func humanResultPushes(r *HumanPvpRoom, id string) []Push {
	box := map[string]any{"__custom_type": "box.box", "materials": map[string]any{}}
	extra := map[string]any{"dungeon_id": r.Dungeon, "client_authoritative": true, "verified": false, "shared_inputs": true, "both_results_agree": r.Reason != "player_forfeit"}
	if r.Native != nil {
		extra["client_authoritative"] = false
		extra["server_authoritative"] = true
		extra["verified"] = true
	}
	if r.Reason == "player_forfeit" {
		extra["reason"] = r.Reason
	}
	result := []Push{push("Avatar", "battle_result", id == r.Winner, box, map[string]any{}, extra)}
	if receipt := r.Players[id].Result; receipt != nil {
		result = append([]Push{push("Avatar", "client_prop_changed", []any{"sync_pvp_score", receipt.OwnScore})}, result...)
		player := r.Players[id]
		result = append([]Push{push("Avatar", "client_prop_changed", []any{"sync_pvp_highest_score", player.AfterHighest}), push("Avatar", "client_prop_changed", []any{"sync_pvp_continues_win_count", player.AfterStreak}), push("Avatar", "client_prop_changed", []any{"sync_weekly_win_count", player.AfterWins})}, result...)
		result = append(result, push("Avatar", "sync_pvp_battle_result", receipt.DeltaScore, receipt.DeltaCoin, receipt.WinContinuously, receipt.DivisionUpdated, receipt.OwnScore, receipt.EnemyScore))
	}
	return append(humanForfeitEndPushes(r), result...)
}

// 恢复沿原UUID、原seed、原冻结阵容及原输入日志执行，不重新计算一场胜负。
func (s *Service) humanResume(ctx context.Context, c *Connection) (bool, []Push, error) {
	if c.SelectedAvatarUnsafe().Progress.Social.HumanRoom == nil {
		return false, nil, nil
	}
	r, err := s.humanTransaction(ctx, c, func(r *HumanPvpRoom, v map[string]*Avatar, id string) error {
		if r.Status == "select" || r.Status == "settled" || r.Status == "aborted" {
			return nil
		}
		player := r.Players[id]
		player.Loaded = false
		player.Ready = false
		player.Started = false
		r.Players[id] = player
		b := v[id].Progress.Battle
		if b == nil || b.UUID != r.UUID {
			return errors.New("真人恢复会话缺失")
		}
		b.BridgeReady = false
		b.BridgeStarted = false
		b.LastSequence = 0
		b.EventLog = nil
		b.Loaded = false
		b.Started = false
		delete(r.Inputs, id)
		if r.Native != nil {
			client := r.Native.Clients[id]
			client.Generation++
			client.Mapping = map[string]string{}
			client.Frame = nil
			client.Checkpoint = nativeengine.Checkpoint{Generation: client.Generation}
			client.CanonicalCursor = 0
			client.WindowHead = ""
			r.Native.Clients[id] = client
			r.Native.WindowDeadline = 0
		} else if r.Status == "fighting" {
			r.Recovering[id] = len(r.Commands)
		}
		return nil
	})
	if err != nil {
		return true, nil, err
	}
	id := hexOf(selectedOID(c))
	c.humanDeliveryUUID = r.UUID
	c.humanDeliverySequence = int64(len(r.Deliveries))
	c.battleStartSent = false
	switch r.Status {
	case "select":
		return true, []Push{humanSelection(r)}, nil
	case "settled":
		return true, humanResultPushes(r, id), nil
	case "aborted":
		return true, humanAbortPushes(), nil
	}
	return true, humanLoadingPushes(r, id), nil
}

// 邀请仅保留原生CHALLENGE_WAIT_TIME窗口；时钟回退不延长邀请。
func (s *Service) tickHumanPvp(ctx context.Context, c *Connection) []Push {
	if c.phase != Playing {
		return nil
	}
	if err := s.refreshHumanPresence(ctx, c); err != nil {
		log.Printf("真人在线租约刷新失败，将重试：%v", err)
	}
	if _, persistent := s.Accounts.(HumanAvatarSnapshotAccounts); persistent {
		p := c.SelectedAvatarUnsafe().Progress
		if p.Social.HumanRoom == nil && (p.SyncPvpMatch == nil || p.SyncPvpMatch.Status != syncPvpMatchWaiting) {
			if s.Now().UnixMilli() < c.nextBackgroundSnapshot {
				return nil
			}
			c.nextBackgroundSnapshot = s.Now().UnixMilli() + 1000
		}
	}
	beforeVisible := mustJSON(SocialProperties(c.SelectedAvatarUnsafe().Progress.Social))
	if err := s.refreshHumanConnection(ctx, c); err != nil {
		log.Printf("真人持久房间读取失败，将重试：%v", err)
		return nil
	}
	visible := []Push{}
	if !bytes.Equal(beforeVisible, mustJSON(SocialProperties(c.SelectedAvatarUnsafe().Progress.Social))) {
		visible = s.socialLivePushes(c.SelectedAvatarUnsafe().Progress.Social)
	}
	if match := c.SelectedAvatarUnsafe().Progress.SyncPvpMatch; match != nil && match.Status == syncPvpMatchWaiting {
		pushes, err := s.tickHumanMatch(ctx, c)
		if err != nil {
			log.Printf("真人匹配事务失败，将重试：%v", err)
		}
		if len(pushes) > 0 {
			return append(visible, pushes...)
		}
	}
	state := c.SelectedAvatarUnsafe().Progress.Social
	expired := false
	for _, invite := range state.Challenges {
		expired = expired || s.Now().Unix() < invite.Time || s.Now().Unix()-invite.Time > int64(socialCatalog.Constants["CHALLENGE_WAIT_TIME"])
	}
	if expired {
		if err := s.updateProgress(ctx, c, func(p *Progress) error {
			for id, invite := range p.Social.Challenges {
				if s.Now().Unix() < invite.Time || s.Now().Unix()-invite.Time > int64(socialCatalog.Constants["CHALLENGE_WAIT_TIME"]) {
					delete(p.Social.Challenges, id)
				}
			}
			return nil
		}); err == nil {
			return s.socialLivePushes(c.SelectedAvatarUnsafe().Progress.Social)
		}
	}
	s.tickNativePvp(ctx, c)
	return visible
}

// 真人优先匹配实际在线、已排队玩家。窗口使用Android下限/扩大量/上限，排序为等候时长与OID。
// level_rate 作为匹配距离的等级权重是本复刻服政策，原厂匹配公式未在Android客户端包含。
func (s *Service) tickHumanMatch(ctx context.Context, c *Connection) ([]Push, error) {
	if err := s.refreshPvpAwards(ctx, c); err != nil {
		return nil, err
	}
	ownID := hexOf(selectedOID(c))
	own := c.SelectedAvatarUnsafe()
	match := own.Progress.SyncPvpMatch
	if match == nil || match.Status != syncPvpMatchWaiting {
		return nil, nil
	}
	elapsed := s.Now().Unix() - match.CreatedAt
	if elapsed < 0 {
		return nil, errors.New("同步匹配时钟回退")
	}
	rule := syncPvpRule()
	if elapsed < int64(rule.MinMatchTime) {
		return nil, nil
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持双端匹配")
	}
	online, err := s.humanOnlineIDs(ctx)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for id, active := range online {
		if active && id != ownID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	window := 400
	if len(rule.MaxMatchScore) == 3 {
		window = rule.MaxMatchScore[0] + int(elapsed)*rule.MaxMatchScore[1]
		if window > rule.MaxMatchScore[2] {
			window = rule.MaxMatchScore[2]
		}
	}
	candidates := []Avatar{}
	for begin := 0; begin < len(ids); begin += 1000 {
		end := begin + 1000
		if end > len(ids) {
			end = len(ids)
		}
		rows, err := store.SocialAvatars(ctx, SocialSearch{OIDs: ids[begin:end], Limit: 1000})
		if err != nil {
			return nil, err
		}
		for _, peer := range rows {
			waiting := peer.Progress.SyncPvpMatch
			if waiting == nil || waiting.Status != syncPvpMatchWaiting || peer.Progress.Battle != nil && !peer.Progress.Battle.Finished || humanRoomActive(peer.Progress.Social.HumanRoom) {
				continue
			}
			distance := math.Abs(float64(waiting.Score-match.Score)) + math.Abs(float64(peer.Info.Level-own.Info.Level))*rule.LevelRate
			if distance <= float64(window) && s.Now().Unix()-waiting.CreatedAt >= int64(rule.MinMatchTime) {
				candidates = append(candidates, peer)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i].Progress.SyncPvpMatch, candidates[j].Progress.SyncPvpMatch
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return hexOf(candidates[i].OID) < hexOf(candidates[j].OID)
	})
	for _, peer := range candidates {
		peerID := hexOf(peer.OID)
		var room *HumanPvpRoom
		rows, err := store.UpdateSocial(ctx, []string{ownID, peerID}, func(v map[string]*Avatar) error {
			for _, id := range []string{ownID, peerID} {
				av := v[id]
				if av == nil || av.Progress.SyncPvpMatch == nil || av.Progress.SyncPvpMatch.Status != syncPvpMatchWaiting || av.Progress.Battle != nil && !av.Progress.Battle.Finished || humanRoomActive(av.Progress.Social.HumanRoom) {
					return errors.New("匹配候选已被其他房间取走")
				}
				ensureSocial(&av.Progress.Social)
			}
			var err error
			room, err = s.newHumanRoom(v, []string{ownID, peerID}, true)
			return err
		})
		if err != nil {
			continue
		}
		acceptSocialRows(c, rows)
		_ = room
		return nil, nil
	}
	// Android分段 lose_match_time 提供机器人兜底等待范围，选用上界避免先到用户跳过真人机会。
	scoreRule, err := syncPvpScoreRuleFor(match.Score)
	if err != nil {
		return nil, err
	}
	fallback := rule.MinMatchTime
	if len(scoreRule.LoseMatchTime) == 2 {
		fallback = scoreRule.LoseMatchTime[1]
	}
	if fallback < rule.MinMatchTime {
		fallback = rule.MinMatchTime
	}
	if elapsed < int64(fallback) {
		return nil, nil
	}
	var robot *SyncPvpMatch
	if err = s.updateProgress(ctx, c, func(p *Progress) error {
		if p.SyncPvpMatch == nil || p.SyncPvpMatch.Status != syncPvpMatchWaiting || p.SyncPvpMatch.BattleUUID != match.BattleUUID {
			return errors.New("匹配等待状态已改变")
		}
		p.SyncPvpMatch = nil
		var e error
		robot, e = prepareSyncPvpMatch(p, match.BattleUUID, s.Now())
		if e != nil {
			return e
		}
		av := c.SelectedAvatarUnsafe()
		av.Progress = *p
		robot.OwnProfile = socialProfile(av)
		return nil
	}); err != nil {
		return nil, err
	}
	infos, err := s.syncPvpSelectInfos(c, robot)
	if err != nil {
		return nil, err
	}
	robotRule, err := syncPvpScoreRuleFor(robot.Score)
	if err != nil {
		return nil, err
	}
	return []Push{push("Avatar", "select_pvp_cards", infos, robotRule.LineupNum)}, nil
}
func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
