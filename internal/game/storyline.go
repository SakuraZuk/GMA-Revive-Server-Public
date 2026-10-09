package game

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Android71BB353A的finished_storyline以call_server追加callback_id。
// 885071C3的剧情桶为extra_key/storylines/dungeons；只保存观影状态，绝不授权资产。
func storylineProperties(p Progress) map[string]any {
	names := append([]string{}, p.PlayedStorylines...)
	return map[string]any{"extra_key": "storyline", "storylines": names, "dungeons": []int{}}
}

func storylinePush(p Progress) Push {
	return push("Avatar", "client_prop_set", []any{"extra_info_mgr", "storyline", storylineProperties(p)})
}

func (s *Service) finishedStoryline(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 2 {
		return nil, errors.New("剧情完成上报需要玩家状态、回调编号及剧情名")
	}
	var callback int
	var name string
	if json.Unmarshal(args[0], &callback) != nil || callback <= 0 {
		return nil, errors.New("剧情完成回调编号必须为正整数")
	}
	if json.Unmarshal(args[1], &name) != nil || len(name) == 0 || len(name) > 256 || !utf8.ValidString(name) || strings.ContainsRune(name, utf8.RuneError) {
		return nil, errors.New("剧情名必须为有效UTF-8且不超过256字节")
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return nil, errors.New("剧情名不能包含控制字符")
		}
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		for _, previous := range p.PlayedStorylines {
			if previous == name {
				return nil
			}
		}
		if len(p.PlayedStorylines) >= 8192 {
			return errors.New("剧情播放记录超出安全容量")
		}
		p.PlayedStorylines = append(p.PlayedStorylines, name)
		return nil
	}); err != nil {
		return nil, err
	}
	// 先刷新原生桶，再完成回调；与参考剧情重载检查顺序一致。
	return []Push{storylinePush(c.SelectedAvatarUnsafe().Progress), Callback(callback, []any{RetSuccess})}, nil
}
