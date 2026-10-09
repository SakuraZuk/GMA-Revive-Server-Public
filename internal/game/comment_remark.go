package game

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) cardRemarkRPC(ctx context.Context, c *Connection, store SocialAccounts, method string, args []json.RawMessage) ([]Push, error) {
	if method == "query_card_remark" {
		var card int
		if len(args) != 1 || json.Unmarshal(args[0], &card) != nil || !socialCardKnown(card) {
			return nil, errors.New("幻书评价查询参数无效")
		}
		stats, err := store.CardRemark(ctx, card)
		if err != nil {
			return nil, err
		}
		comments, err := store.QueryComments(ctx, card, 0, 2)
		if err != nil {
			return nil, err
		}
		rows := []any{}
		for _, comment := range comments {
			rows = append(rows, comment.wire())
		}
		return []Push{push("Avatar", "on_query_card_remark", card, stats, rows)}, nil
	}
	cb, ok := callbackArg(args)
	var card int
	want := 2
	if method == "select_card_tags" {
		want = 3
	}
	if !ok || len(args) != want || json.Unmarshal(args[1], &card) != nil || !socialCardKnown(card) {
		return nil, errors.New("幻书评价签名无效")
	}
	var tags []int
	if method == "select_card_tags" {
		if json.Unmarshal(args[2], &tags) != nil {
			return nil, errors.New("幻书标签容器无效")
		}
		var config struct {
			Fight     []int `json:"fight_tags"`
			Character []int `json:"character_tags"`
		}
		if json.Unmarshal(socialCatalog.Tables["cards"][intString(card)], &config) != nil {
			return nil, errors.New("幻书标签目录缺失")
		}
		allowed := map[int]bool{}
		for _, id := range append(config.Fight, config.Character...) {
			allowed[id] = true
		}
		seen := map[int]bool{}
		for _, id := range tags {
			if !allowed[id] || seen[id] {
				return []Push{Callback(cb, []any{socialCatalog.Errors["RET_ARCHIVE_TAG_NOT_EXIST"]})}, nil
			}
			seen[id] = true
		}
	}
	ownID := hexOf(selectedOID(c))
	rows, err := store.UpdateSocial(ctx, []string{ownID}, func(avatars map[string]*Avatar) error {
		p := &avatars[ownID].Progress
		ensureSocial(&p.Social)
		remark := p.Social.Remarks[card]
		switch method {
		case "select_card_tags":
			remark.Tags = append([]int{}, tags...)
		case "like_card_remark":
			if remark.Like {
				return socialFailure("RET_ARCHIVE_HAS_LIKE", "已经点赞该幻书")
			}
			remark.Like = true
		case "unlike_card_remark":
			if !remark.Like {
				return socialFailure("RET_ARCHIVE_NOT_LIKE", "尚未点赞该幻书")
			}
			remark.Like = false
		default:
			return errors.New("未知幻书评价操作")
		}
		p.Social.Remarks[card] = remark
		p.Social.Revision++
		return nil
	})
	if err != nil {
		var se socialError
		if errors.As(err, &se) {
			return []Push{Callback(cb, []any{se.code})}, nil
		}
		return nil, err
	}
	av := rows[0]
	for i, current := range c.identity.Avatars {
		if current.Hostnum == c.hostnum {
			c.identity.Avatars[i].Progress = av.Progress
		}
	}
	c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
	return append(s.socialLivePushes(av.Progress.Social), Callback(cb, []any{RetSuccess})), nil
}

// 在线跨北京时间零点刷新评论次数，数据库存档与客户端门槛同时更新。
func (s *Service) refreshSocialDaily(ctx context.Context, c *Connection) ([]Push, error) {
	if c.phase != Playing {
		return nil, nil
	}
	p := c.SelectedAvatarUnsafe().Progress
	if p.Social.CommentDay == "" || p.Social.CommentDay >= socialDay(s.Now()) {
		return nil, nil
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		day := socialDay(s.Now())
		if p.Social.CommentDay > day {
			return errors.New("社交刷新业务时钟回拨")
		}
		if p.Social.CommentDay < day {
			p.Social.CommentDay = day
			p.Social.CommentCounts = map[int]int{}
			p.Social.Revision++
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return []Push{push("Avatar", "client_prop_changed", []any{"archive_card_2_comment_times", map[int]int{}})}, nil
}
