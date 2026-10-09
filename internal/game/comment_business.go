package game

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

func (s *Service) commentRPC(ctx context.Context, c *Connection, store SocialAccounts, method string, args []json.RawMessage) ([]Push, error) {
	if method == "query_card_comments" {
		var card, start, order int
		if len(args) != 3 || json.Unmarshal(args[0], &card) != nil || json.Unmarshal(args[1], &start) != nil || json.Unmarshal(args[2], &order) != nil || start < 0 || start > 10000 || (order != 1 && order != 2) || !socialCardKnown(card) {
			return nil, errors.New("评论查询参数无效")
		}
		rows, err := store.QueryComments(ctx, card, start, order)
		if err != nil {
			return nil, err
		}
		list := []any{}
		for _, row := range rows {
			list = append(list, row.wire())
		}
		return []Push{push("Avatar", "on_query_card_comments", card, start, order, list)}, nil
	}
	cb, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("评论回调参数无效")
	}
	var card int
	var id, content string
	want := 3
	if method == "update_card_comment" {
		want = 4
	}
	if len(args) != want || json.Unmarshal(args[1], &card) != nil || !socialCardKnown(card) {
		return nil, errors.New("评论幻书编号或签名无效")
	}
	if method == "send_card_comment" {
		if json.Unmarshal(args[2], &content) != nil {
			return nil, errors.New("评论内容类型无效")
		}
		id = newBattleUUID(s.Now())
	} else {
		if json.Unmarshal(args[2], &id) != nil {
			return nil, errors.New("评论标识类型无效")
		}
		if _, err := SocialOID(id); err != nil {
			return nil, err
		}
		if method == "update_card_comment" && json.Unmarshal(args[3], &content) != nil {
			return nil, errors.New("评论内容类型无效")
		}
	}
	if method == "send_card_comment" || method == "update_card_comment" {
		if !utf8.ValidString(content) || utf8.RuneCountInString(content) > socialCatalog.Constants["ARCHIVE_CONTENT_MAX_LEN"] {
			return []Push{Callback(cb, []any{socialCatalog.Errors["RET_ARCHIVE_CONTENT_TOO_LONG"], ObjectID(id)})}, nil
		}
		if strings.TrimSpace(content) == "" {
			return []Push{Callback(cb, []any{socialCatalog.Errors["RET_ARCHIVE_CONTENT_EMPTY"], ObjectID(id)})}, nil
		}
	}
	av, comment, err := store.UpdateComment(ctx, selectedOID(c), id, func(av *Avatar, row *CardComment) error {
		state := &av.Progress.Social
		ensureSocial(state)
		self := hexOf(av.OID)
		if method != "send_card_comment" && (row.Owner == "" || row.Deleted || row.CardID != card) {
			return socialFailure("RET_ARCHIVE_COMMENT_NOT_EXIST", "评论不存在或已删除")
		}
		switch method {
		case "send_card_comment":
			day := socialDay(s.Now())
			if state.CommentDay > day {
				return errors.New("评论业务时钟回拨")
			}
			if state.CommentDay != day {
				state.CommentDay = day
				state.CommentCounts = map[int]int{}
			}
			if state.CommentCounts[card] >= socialCatalog.Constants["ARCHIVE_DAILY_MAX_COMMENT_COUNT"] {
				return socialFailure("RET_ARCHIVE_COMMENT_REACH_MAX_TIMES", "今日该幻书评论达到上限")
			}
			*row = CardComment{ID: id, Owner: self, CardID: card, Content: content, CreatedAt: s.Now().Unix(), Profile: socialProfile(*av), Likes: map[string]bool{}}
			state.CommentCounts[card]++
			remark := state.Remarks[card]
			remark.CommentIDs = append(remark.CommentIDs, id)
			state.Remarks[card] = remark
			state.Comments[id] = *row
		case "update_card_comment":
			if row.Owner != self {
				return socialFailure("RET_ARCHIVE_COMMENT_NOT_EXIST", "不能修改他人评论")
			}
			row.Content = content
			row.Profile = socialProfile(*av)
			state.Comments[id] = *row
		case "remove_card_comment":
			if row.Owner != self {
				return socialFailure("RET_ARCHIVE_COMMENT_NOT_EXIST", "不能删除他人评论")
			}
			row.Deleted = true
			remark := state.Remarks[card]
			ids := []string{}
			for _, existing := range remark.CommentIDs {
				if existing != id {
					ids = append(ids, existing)
				}
			}
			remark.CommentIDs = ids
			state.Remarks[card] = remark
			state.Comments[id] = *row
		case "like_card_comment":
			if row.Likes == nil {
				row.Likes = map[string]bool{}
			}
			if row.Likes[self] {
				return socialFailure("RET_ARCHIVE_HAS_LIKE", "已经点赞")
			}
			row.Likes[self] = true
			state.LikedComments[id] = card
		case "unlike_card_comment":
			if !row.Likes[self] {
				return socialFailure("RET_ARCHIVE_NOT_LIKE", "尚未点赞")
			}
			delete(row.Likes, self)
			delete(state.LikedComments, id)
		default:
			return errors.New("未知评论操作")
		}
		state.Revision++
		return nil
	})
	if err != nil {
		var se socialError
		if errors.As(err, &se) {
			values := []any{se.code}
			if method == "send_card_comment" {
				values = append(values, ObjectID(id))
			}
			return []Push{Callback(cb, values)}, nil
		}
		return nil, err
	}
	for i, current := range c.identity.Avatars {
		if current.Hostnum == c.hostnum {
			c.identity.Avatars[i].Progress = av.Progress
		}
		c.pendingSocialOIDs = append(c.pendingSocialOIDs, append([]byte{}, av.OID...))
	}
	pushes := s.socialLivePushes(av.Progress.Social)
	values := []any{RetSuccess}
	if method == "send_card_comment" {
		values = append(values, ObjectID(comment.ID))
	}
	return append(pushes, Callback(cb, values)), nil
}
func socialCardKnown(card int) bool {
	_, ok := socialCatalog.Tables["cards"][intString(card)]
	return ok
}
