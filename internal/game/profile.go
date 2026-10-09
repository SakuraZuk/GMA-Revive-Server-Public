package game

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 客户端 error_code 875C5BCF 与 avatar_info_check B41789AA 的返回码。
const (
	RetNicknameLength    = 10034
	RetNicknameExists    = 10035
	RetNicknameCharacter = 10039
)

var ErrNicknameExists = errors.New("名字已被使用或角色已经取名")
var ErrProfileState = errors.New("角色不在创建引导中")

type ProfileAccounts interface {
	SetNicknameGender(context.Context, []byte, string, int) (Avatar, error)
}

var nicknamePattern = regexp.MustCompile("^[" + clientBaseline.Naming.Words + strings.ReplaceAll(regexp.QuoteMeta(clientBaseline.Naming.Special), "-", `\-`) + "]+$")

func ValidateNickname(nickname string) int {
	n := utf8.RuneCountInString(nickname)
	if n < clientBaseline.Naming.Min || n > clientBaseline.Naming.Max {
		return RetNicknameLength
	}
	if !utf8.ValidString(nickname) || !nicknamePattern.MatchString(nickname) {
		return RetNicknameCharacter
	}
	return RetSuccess
}

// avatar.py 两个语音接口均由 call_server 包装；其 callback 为 (Bool,String)。
func (s *Service) setVoice(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("语音设置需要玩家状态及回调和语种")
	}
	var cb, value int
	if json.Unmarshal(args[0], &cb) != nil || json.Unmarshal(args[1], &value) != nil {
		return nil, errors.New("语音设置参数必须是整数")
	}
	if value != 1 && value != 2 && !(method == "set_global_vo" && value == 0) {
		return []Push{Callback(cb, []any{false, "语音设置无效"})}, nil
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if method == "set_global_vo" {
			p.GlobalVO = value
		} else {
			p.StoryVO = value
		}
		return nil
	}); err != nil {
		return []Push{Callback(cb, []any{false, "语音设置保存失败"})}, nil
	}
	field := "story_vo"
	if method == "set_global_vo" {
		field = "global_vo"
	}
	return []Push{push("Avatar", "client_prop_changed", []any{field, value}), Callback(cb, []any{true, ""})}, nil
}

func (s *Service) setNicknameGender(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("取名需要玩家状态及回调、名称、性别三个参数")
	}
	var cb, gender int
	var name string
	if json.Unmarshal(args[0], &cb) != nil || json.Unmarshal(args[1], &name) != nil || json.Unmarshal(args[2], &gender) != nil || (gender != 1 && gender != 2) {
		return nil, errors.New("取名参数类型或性别无效")
	}
	if code := ValidateNickname(name); code != 0 {
		return []Push{Callback(cb, []any{code})}, nil
	}
	store, ok := s.Accounts.(ProfileAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持角色资料")
	}
	for i, av := range c.identity.Avatars {
		if av.Hostnum != c.hostnum {
			continue
		}
		updated, err := store.SetNicknameGender(ctx, av.OID, name, gender)
		if errors.Is(err, ErrNicknameExists) {
			return []Push{Callback(cb, []any{RetNicknameExists})}, nil
		}
		if err != nil {
			return nil, err
		}
		c.identity.Avatars[i] = updated
		// client_prop_changed 注册为 Tuple；字段描述器触发 UI 通知。必须先同步再回调。
		return []Push{push("Avatar", "client_prop_changed", []any{"nickname", name}), push("Avatar", "client_prop_changed", []any{"gender", gender}), push("Avatar", "client_prop_changed", []any{"nickname_flag", true}), Callback(cb, []any{RetSuccess})}, nil
	}
	return nil, ErrProfileState
}

func (a *FixtureAccounts) SetNicknameGender(ctx context.Context, oid []byte, name string, gender int) (Avatar, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Avatar{}, err
	}
	if ValidateNickname(name) != 0 || (gender != 1 && gender != 2) {
		return Avatar{}, errors.New("取名参数无效")
	}
	for account, record := range a.records {
		for i, av := range record.Avatars {
			if string(av.OID) != string(oid) {
				continue
			}
			if av.NicknameSet {
				if av.Info.Nickname == name && av.Gender == gender {
					return av, nil
				}
				return Avatar{}, ErrNicknameExists
			}
			if av.Progress.GuideTasks[1000].Status != 1 {
				return Avatar{}, ErrProfileState
			}
			for _, other := range a.records {
				for _, candidate := range other.Avatars {
					if candidate.Hostnum == av.Hostnum && candidate.NicknameSet && candidate.Info.Nickname == name {
						return Avatar{}, ErrNicknameExists
					}
				}
			}
			av.Info.Nickname, av.Gender, av.NicknameSet = name, gender, true
			record.Avatars[i] = av
			a.records[account] = record
			return av, nil
		}
	}
	return Avatar{}, ErrProfileState
}
