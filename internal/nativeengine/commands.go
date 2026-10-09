package nativeengine

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrUnmappedEntity = errors.New("客户端尚未证明原生实体映射")

type Update struct {
	Status string  `json:"status"`
	Steps  int64   `json:"steps"`
	State  State   `json:"state"`
	Events []Event `json:"events"`
	Result *Result `json:"result"`
}

type State struct {
	Units           []UnitSnapshot `json:"units"`
	CurrentInputEID string         `json:"current_input_eid"`
	CurrentMaster   any            `json:"current_master"`
	AwaitingPlayer  bool           `json:"awaiting_player"`
	InputTime       float64        `json:"player_input_time"`
	ActionCounter   int64          `json:"action_counter"`
}

type Event struct {
	Kind string         `json:"kind"`
	Data map[string]any `json:"data"`
}

type Result struct {
	Winners []string `json:"winner_eids"`
}

func ParseUpdate(raw json.RawMessage) (*Update, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var update Update
	if err := decoder.Decode(&update); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("原生状态回复包含多余JSON内容")
	}
	if update.Status != "running" && update.Status != "finished" {
		return nil, errors.New("原生回复缺少有效战斗状态")
	}
	if len(update.State.Units) > 512 {
		return nil, errors.New("原生快照单位数量超过保护上限")
	}
	return &update, nil
}

func normalizedMaster(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	if raw, ok := value.(map[string]any); ok {
		if s, ok := raw["bytes_hex"].(string); ok {
			decoded, err := hex.DecodeString(s)
			if err == nil && len(decoded) == 24 {
				return string(decoded)
			}
			return s
		}
	}
	return ""
}

func unitCamp(u UnitSnapshot) int {
	camp, _ := identityInteger(unitValue(u, "camp", unitValue(u, "camp_id", nil)))
	return camp
}

func TurnOwner(state State, owners [2]string) string {
	master := normalizedMaster(state.CurrentMaster)
	if master != "" && (master == owners[0] || master == owners[1]) {
		return master
	}
	for _, u := range state.Units {
		if identityEID(u["eid"]) == state.CurrentInputEID {
			switch unitCamp(u) {
			case 1:
				return owners[0]
			case 2:
				return owners[1]
			}
			break
		}
	}
	return ""
}

func findUnit(units []UnitSnapshot, eid string) (UnitSnapshot, error) {
	var result UnitSnapshot
	for _, u := range units {
		if identityEID(u["eid"]) == eid {
			if result != nil {
				return nil, errors.New("原生实体EID重复")
			}
			result = u
		}
	}
	if result == nil {
		return nil, fmt.Errorf("原生命令实体不在快照内：%s", eid)
	}
	return result, nil
}

// Playback是已按该接收端固定映射转换的原生RPC描述符；Master由游戏层
// 恢复ObjectID类型。命令来自原生sync_command，不能把未执行的上行点击广播。
type Playback struct {
	Method string `json:"method"`
	Args   []any  `json:"args"`
	Master string `json:"master,omitempty"`
}

// PlaybackExpectation把已经按接收端映射的RPC还原为该客户端log_command会
// 上报的命令形态。播放屏障比较客户端实体与坐标，不能拿原生EID直接比较。
func PlaybackExpectation(play *Playback) (eid string, raw any, roundEID string, err error) {
	if play == nil {
		return "", nil, "", errors.New("播放描述为空")
	}
	switch play.Method {
	case "revival_play_native_command":
		if len(play.Args) != 2 {
			return "", nil, "", errors.New("强制播放描述无效")
		}
		eid, _ = play.Args[0].(string)
		raw = play.Args[1]
		return eid, raw, eid, nil
	case "revival_play_support_command":
		if len(play.Args) != 3 {
			return "", nil, "", errors.New("援护播放描述无效")
		}
		roundEID, _ = play.Args[0].(string)
		eid, _ = play.Args[1].(string)
		raw = play.Args[2]
		return eid, raw, roundEID, nil
	case "revival_do_command", "revival_do_peer_command":
		if len(play.Args) != 2 {
			return "", nil, "", errors.New("普通播放描述无效")
		}
		name, _ := play.Args[0].(string)
		blob, ok := play.Args[1].([]any)
		if name == "" || !ok || len(blob) < 2 {
			return "", nil, "", errors.New("普通播放命令参数无效")
		}
		start := 0
		if marker, ok := blob[0].(string); ok && (marker == "ck_monster" || marker == "ck_skill" || marker == "ck_head") {
			start = 1
		}
		if len(blob) <= start+1 {
			return "", nil, "", errors.New("普通播放命令缺少实体与参数")
		}
		eid, _ = blob[start].(string)
		args := append([]any(nil), blob[start+1:]...)
		if name == "use_skill" && len(args) > 2 {
			args = args[:2]
		}
		return eid, []any{name, args}, eid, nil
	default:
		return "", nil, "", errors.New("未知播放方法")
	}
}

func MapPlayback(event Event, state State, owners [2]string, recipient string, mapping map[string]string, frame *CoordinateFrame) (*Playback, error) {
	if event.Kind != "sync_command" {
		return nil, nil
	}
	args, ok := event.Data["args"].([]any)
	if !ok || len(args) < 3 {
		return nil, errors.New("原生同步命令结构无效")
	}
	eid := identityEID(args[1])
	unit, err := findUnit(state.Units, eid)
	if err != nil {
		return nil, err
	}
	typ, _ := identityInteger(unit["native_type"])
	if unit["kind"] == "field" || typ == 4 {
		return nil, nil
	}
	owner := TurnOwner(State{CurrentInputEID: eid, Units: []UnitSnapshot{unit}}, owners)
	if owner == "" || recipient != owners[0] && recipient != owners[1] {
		return nil, errors.New("原生命令控制者或接收端不属于共同房间")
	}
	command, ok := args[2].([]any)
	if !ok || len(command) != 2 {
		return nil, errors.New("原生命令名与参数结构无效")
	}
	name, ok := command[0].(string)
	if !ok {
		return nil, errors.New("原生命令名称无效")
	}
	payload, ok := command[1].([]any)
	if !ok {
		return nil, errors.New("原生命令参数不是列表")
	}
	mapped := func(native string) (string, error) {
		if mapping[native] == "" {
			return "", fmt.Errorf("%w：%s", ErrUnmappedEntity, native)
		}
		return mapping[native], nil
	}
	caster, err := mapped(eid)
	if err != nil {
		return nil, err
	}
	coord := func(value any) ([3]int, error) {
		c, ok := cube(value)
		if !ok || frame == nil {
			return [3]int{}, errors.New("原生坐标无效或客户端棋盘尚未固定")
		}
		return frame.NativeToClient(c)
	}
	switch name {
	case "stun", "pass_action", "idle", "no_target_idle", "no_move_idle":
		if len(payload) != 0 {
			return nil, errors.New("原生强制控制命令只能无参数")
		}
		return &Playback{Method: "revival_play_native_command", Args: []any{caster, []any{name, []any{}}}}, nil
	case "use_support_skill", "use_extra_support_skill":
		if len(payload) != 4 {
			return nil, errors.New("原生援护必须保留技能、目标及两个位置")
		}
		skill, ok := identityInteger(payload[0])
		if !ok || skill <= 0 {
			return nil, errors.New("原生援护技能无效")
		}
		var target any
		if payload[1] != nil {
			native, ok := payload[1].(string)
			if !ok || native == "" {
				return nil, errors.New("原生援护目标无效")
			}
			if _, err = findUnit(state.Units, native); err != nil {
				return nil, err
			}
			target, err = mapped(native)
			if err != nil {
				return nil, err
			}
		}
		round, ok := event.Data["round_eid"].(string)
		if !ok || round == "" {
			return nil, errors.New("原生援护未保存被中断回合单位")
		}
		roundUnit, err := findUnit(state.Units, round)
		if err != nil {
			return nil, err
		}
		if TurnOwner(State{CurrentInputEID: round, Units: []UnitSnapshot{roundUnit}}, owners) != owner {
			return nil, errors.New("原生援护与被中断回合控制者不符")
		}
		roundClient, err := mapped(round)
		if err != nil {
			return nil, err
		}
		targetPos, err := coord(payload[2])
		if err != nil {
			return nil, err
		}
		attackPos, err := coord(payload[3])
		if err != nil {
			return nil, err
		}
		return &Playback{Method: "revival_play_support_command", Args: []any{roundClient, caster, []any{name, []any{skill, target, targetPos, attackPos}}}}, nil
	case "move_to":
		if len(payload) != 1 {
			return nil, errors.New("原生移动参数数量无效")
		}
		position, err := coord(payload[0])
		if err != nil {
			return nil, err
		}
		method := "revival_do_command"
		if recipient != owner {
			method = "revival_do_peer_command"
		}
		return &Playback{Method: method, Master: owner, Args: []any{"move_to", []any{caster, position}}}, nil
	case "use_skill":
		if len(payload) < 2 {
			return nil, errors.New("原生技能参数缺失")
		}
		skill, ok := identityInteger(payload[0])
		if !ok || skill <= 0 {
			return nil, errors.New("原生技能编号无效")
		}
		target := payload[1]
		if target == nil && len(payload) > 2 {
			target = payload[2]
		}
		if target != nil {
			if native, ok := target.(string); ok {
				if _, err = findUnit(state.Units, native); err != nil {
					return nil, err
				}
				target, err = mapped(native)
				if err != nil {
					return nil, err
				}
			} else {
				target, err = coord(target)
				if err != nil {
					return nil, err
				}
			}
		}
		method := "revival_do_command"
		if recipient != owner {
			method = "revival_do_peer_command"
		}
		return &Playback{Method: method, Master: owner, Args: []any{"use_skill", []any{"ck_monster", caster, skill, target}}}, nil
	default:
		return nil, fmt.Errorf("原生命令尚无已取证播放方法：%s", name)
	}
}

// MapClick只转换当前鉴权控制者的点击；是否可移动/技能范围/援护次数
// 仍由单个原生server_battle验证。客户端坐标与EID不得直通引擎。
func MapClick(name string, blob []any, state State, owners [2]string, authenticated string, mapping map[string]string, frame *CoordinateFrame) (map[string]any, error) {
	if owners[0] == "" || owners[1] == "" || owners[0] == owners[1] || authenticated == "" || !state.AwaitingPlayer || state.CurrentInputEID == "" || TurnOwner(state, owners) != authenticated {
		return nil, errors.New("当前鉴权玩家没有原生输入窗口")
	}
	inverse := map[string]string{}
	for n, c := range mapping {
		if c == "" || inverse[c] != "" {
			return nil, errors.New("客户端实体映射不是一对一")
		}
		inverse[c] = n
	}
	if len(blob) > 0 {
		if marker, ok := blob[0].(string); ok && (marker == "ck_monster" || marker == "ck_skill" || marker == "ck_head") {
			blob = blob[1:]
		}
	}
	coord := func(value any) ([3]int, error) {
		c, ok := cube(value)
		if !ok || frame == nil {
			return [3]int{}, errors.New("客户端坐标无效或棋盘尚未固定")
		}
		return frame.ClientToNative(c)
	}
	mapped := func(value any) (string, error) {
		client, ok := value.(string)
		if !ok || inverse[client] == "" {
			return "", errors.New("点击实体没有已证明原生映射")
		}
		return inverse[client], nil
	}
	switch name {
	case "move_to":
		if len(blob) != 2 {
			return nil, errors.New("原生移动点击需要单位及位置")
		}
		caster, err := mapped(blob[0])
		if err != nil {
			return nil, err
		}
		if caster != state.CurrentInputEID {
			return nil, errors.New("移动点击不是当前原生单位")
		}
		position, err := coord(blob[1])
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": name, "args": []any{caster, position}}, nil
	case "use_skill":
		if len(blob) != 3 {
			return nil, errors.New("原生技能点击需要单位、技能及目标")
		}
		caster, err := mapped(blob[0])
		if err != nil {
			return nil, err
		}
		unit, err := findUnit(state.Units, caster)
		if err != nil {
			return nil, err
		}
		if TurnOwner(State{CurrentInputEID: caster, Units: []UnitSnapshot{unit}}, owners) != authenticated {
			return nil, errors.New("技能单位不受鉴权玩家控制")
		}
		if caster != state.CurrentInputEID {
			support, _ := identityFlag(unitValue(unit, "support", false))
			extra, _ := identityInteger(unitValue(unit, "extra_support_skill", 0))
			if !support && extra == 0 {
				return nil, errors.New("非当前单位也不是原生援护者")
			}
		}
		skill, ok := identityInteger(blob[1])
		if !ok || skill <= 0 {
			return nil, errors.New("点击技能编号无效")
		}
		target := blob[2]
		if target != nil {
			if _, ok := target.(string); ok {
				target, err = mapped(target)
				if err != nil {
					return nil, err
				}
				if _, err = findUnit(state.Units, target.(string)); err != nil {
					return nil, err
				}
			} else {
				target, err = coord(target)
				if err != nil {
					return nil, err
				}
			}
		}
		return map[string]any{"name": name, "args": []any{caster, skill, target}}, nil
	default:
		return nil, errors.New("PVP仅接受已取证移动与技能点击")
	}
}
