package nativeengine

import (
	"encoding/json"
	"errors"
)

// Checkpoint是持久播放屏障，客户端只确认已按顺序播放。
// 它不能产生胜方；TerminalMatches必须传入单原生引擎已经生成的胜方。
type Checkpoint struct {
	Generation       int64              `json:"generation"`
	StartedSequence  int64              `json:"started_sequence"`
	LastSequence     int64              `json:"last_sequence"`
	Commands         []ExpectedPlayback `json:"commands,omitempty"`
	Seen             int                `json:"seen"`
	SeenSequence     int64              `json:"seen_sequence"`
	Boundary         int                `json:"boundary"`
	BoundarySequence int64              `json:"boundary_sequence"`
	ResultCandidate  *ClientFinish      `json:"result_candidate,omitempty"`
}

type ExpectedPlayback struct {
	Key      string `json:"key"`
	RoundEID string `json:"round_eid"`
	Support  bool   `json:"support"`
}

type ClientFinish struct {
	Sequence        int64    `json:"sequence"`
	Generation      int64    `json:"generation"`
	StartedSequence int64    `json:"started_sequence"`
	Seen            int      `json:"seen"`
	Delivered       int      `json:"delivered"`
	CommandSequence int64    `json:"command_sequence"`
	Winners         []string `json:"winner_eids"`
}

// 普通技能只比原生技能与实体/位置目标；原生RPC重新计算攻击站位。
// 援护必须比较全部四参数，强制控制只能保留原无参命令名。
func playbackKey(eid string, raw any) (string, bool, error) {
	command, ok := raw.([]any)
	if !ok || len(command) != 2 || eid == "" {
		return "", false, errors.New("播放确认命令结构无效")
	}
	name, ok := command[0].(string)
	if !ok {
		return "", false, errors.New("播放确认命令名无效")
	}
	args, ok := command[1].([]any)
	if !ok {
		return "", false, errors.New("播放确认命令参数无效")
	}
	var normalized any
	support := false
	switch name {
	case "stun", "pass_action", "idle", "no_target_idle", "no_move_idle":
		if len(args) != 0 {
			return "", false, errors.New("强制控制确认必须无参数")
		}
		normalized = []any{}
	case "move_to":
		if len(args) != 1 {
			return "", false, errors.New("移动确认参数无效")
		}
		position, ok := cube(args[0])
		if !ok {
			return "", false, errors.New("移动确认坐标无效")
		}
		normalized = []any{position}
	case "use_skill":
		if len(args) < 2 {
			return "", false, errors.New("技能确认参数缺失")
		}
		skill, ok := identityInteger(args[0])
		if !ok || skill <= 0 {
			return "", false, errors.New("技能确认编号无效")
		}
		target := args[1]
		if target == nil && len(args) > 2 {
			target = args[2]
		}
		if target != nil {
			if s, ok := target.(string); ok {
				if s == "" {
					return "", false, errors.New("技能确认目标为空")
				}
			} else {
				position, ok := cube(target)
				if !ok {
					return "", false, errors.New("技能确认目标无效")
				}
				target = position
			}
		}
		normalized = []any{skill, target}
	case "use_support_skill", "use_extra_support_skill":
		if len(args) != 4 {
			return "", false, errors.New("援护确认四参数缺失")
		}
		support = true
		skill, ok := identityInteger(args[0])
		if !ok || skill <= 0 {
			return "", false, errors.New("援护确认技能无效")
		}
		if args[1] != nil {
			s, ok := args[1].(string)
			if !ok || s == "" {
				return "", false, errors.New("援护确认目标无效")
			}
		}
		targetPos, ok := cube(args[2])
		if !ok {
			return "", false, errors.New("援护确认目标位置无效")
		}
		attackPos, ok := cube(args[3])
		if !ok {
			return "", false, errors.New("援护确认攻击位置无效")
		}
		normalized = []any{skill, args[1], targetPos, attackPos}
	default:
		return "", false, errors.New("该原生命令没有播放确认规则")
	}
	rawKey, err := json.Marshal([]any{eid, name, normalized})
	return string(rawKey), support, err
}

func (p *Checkpoint) Track(eid string, raw any, roundEID string) error {
	if p.Generation <= 0 || len(p.Commands) >= MaxJournalEntries {
		return errors.New("播放世代无效或日志已达到保护上限")
	}
	key, support, err := playbackKey(eid, raw)
	if err != nil {
		return err
	}
	if support && roundEID == "" {
		return errors.New("援护播放没有被冻结的原回合单位")
	}
	if roundEID == "" {
		roundEID = eid
	}
	p.Commands = append(p.Commands, ExpectedPlayback{Key: key, RoundEID: roundEID, Support: support})
	p.ResultCandidate = nil
	return nil
}

// 只接收当前恢复世代递增事件；握手事件由外层先按原协议检查。
// 旧连接/旧场景事件不能推进重建后的播放游标。
func (p *Checkpoint) Observe(generation, sequence int64, kind, eid string, command any, winners []string) error {
	if generation != p.Generation || generation <= 0 || sequence <= p.LastSequence {
		return errors.New("播放确认世代失配或事件序号回退")
	}
	if kind != "started" && p.StartedSequence == 0 {
		return errors.New("客户端播放尚未开始")
	}
	switch kind {
	case "started":
		if p.StartedSequence != 0 {
			return errors.New("播放开始重复")
		}
		p.StartedSequence = sequence
		p.ResultCandidate = nil
	case "command":
		key, _, err := playbackKey(eid, command)
		if err != nil {
			return err
		}
		if p.Seen >= len(p.Commands) || key != p.Commands[p.Seen].Key {
			return errors.New("播放命令与服务端FIFO日志不符")
		}
		p.Seen++
		p.SeenSequence = sequence
	case "turn", "input", "round_end":
		if p.Seen > p.Boundary && p.Seen <= len(p.Commands) && sequence > p.SeenSequence {
			expected := p.Commands[p.Seen-1]
			if kind == "turn" && !expected.Support || eid == expected.RoundEID && (kind != "round_end" || !expected.Support) {
				p.Boundary = p.Seen
				p.BoundarySequence = sequence
			}
		}
	case "result":
		if p.Seen > 0 && p.Seen == len(p.Commands) {
			p.Boundary = p.Seen
			p.BoundarySequence = sequence
		}
		p.ResultCandidate = &ClientFinish{Sequence: sequence, Generation: p.Generation, StartedSequence: p.StartedSequence, Seen: p.Seen, Delivered: len(p.Commands), CommandSequence: p.SeenSequence, Winners: append([]string(nil), winners...)}
	default:
		return errors.New("该事件不能作为原生播放确认")
	}
	p.LastSequence = sequence
	return nil
}

func (p *Checkpoint) Ready() bool {
	return p.Generation > 0 && p.StartedSequence > 0 && p.Seen == len(p.Commands) && p.Boundary == len(p.Commands)
}

func (p *Checkpoint) TerminalMatches(authoritativeWinners []string, required int) bool {
	c := p.ResultCandidate
	if c == nil || !p.Ready() || required != len(p.Commands) || c.Generation != p.Generation || c.StartedSequence != p.StartedSequence || c.Sequence <= p.StartedSequence || c.Seen != required || c.Delivered != required || c.CommandSequence != p.SeenSequence {
		return false
	}
	if len(c.Winners) != len(authoritativeWinners) {
		return false
	}
	seen := map[string]bool{}
	for _, winner := range c.Winners {
		if winner == "" || seen[winner] {
			return false
		}
		seen[winner] = true
	}
	for _, winner := range authoritativeWinners {
		if !seen[winner] {
			return false
		}
	}
	return true
}
