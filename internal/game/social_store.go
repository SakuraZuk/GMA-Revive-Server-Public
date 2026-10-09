package game

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// SocialAccounts 在双方玩家最新行锁存档内执行关系变更；失败不得部分提交。
type SocialAccounts interface {
	SocialAvatars(context.Context, SocialSearch) ([]Avatar, error)
	UpdateSocial(context.Context, []string, func(map[string]*Avatar) error) ([]Avatar, error)
	UpdateAllSocial(context.Context, func(map[string]*Avatar) error) ([]Avatar, error)
	QueryComments(context.Context, int, int, int) ([]CardComment, error)
	UpdateComment(context.Context, []byte, string, func(*Avatar, *CardComment) error) (Avatar, CardComment, error)
	AsyncPvpRank(context.Context, []byte, int) (int, error)
	CardRemark(context.Context, int) (CardRemarkStats, error)
}
type CardRemarkStats struct {
	CardID   int         `json:"card_id"`
	Tags     map[int]int `json:"tag_2_count"`
	Likes    int         `json:"like_count"`
	Comments int         `json:"comment_count"`
}

func (a *FixtureAccounts) CardRemark(ctx context.Context, card int) (CardRemarkStats, error) {
	if e := ctx.Err(); e != nil {
		return CardRemarkStats{}, e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	r := CardRemarkStats{CardID: card, Tags: map[int]int{}}
	for _, rec := range a.records {
		for _, av := range rec.Avatars {
			remark := av.Progress.Social.Remarks[card]
			if remark.Like {
				r.Likes++
			}
			for _, tag := range remark.Tags {
				r.Tags[tag]++
			}
			for _, comment := range av.Progress.Social.Comments {
				if !comment.Deleted && comment.CardID == card {
					r.Comments++
				}
			}
		}
	}
	return r, nil
}

func (a *FixtureAccounts) AsyncPvpRank(ctx context.Context, oid []byte, host int) (int, error) {
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var own *Avatar
	for _, r := range a.records {
		for _, av := range r.Avatars {
			if string(av.OID) == string(oid) {
				copy := av
				own = &copy
			}
		}
	}
	if own == nil {
		return 0, errors.New("排行角色不存在")
	}
	if own.Progress.AsyncPvp.Score <= 0 {
		return 0, nil
	}
	rank := 1
	for _, r := range a.records {
		for _, av := range r.Avatars {
			if host > 0 && av.Hostnum != host {
				continue
			}
			if av.Progress.AsyncPvp.Score > own.Progress.AsyncPvp.Score || (av.Progress.AsyncPvp.Score == own.Progress.AsyncPvp.Score && hexOf(av.OID) < hexOf(oid)) {
				rank++
			}
		}
	}
	return rank, nil
}

type SocialSearch struct {
	OIDs     []string
	UID      int64
	Hostnum  int
	Nickname string
	Limit    int
}

func (a *FixtureAccounts) SocialAvatars(ctx context.Context, search SocialSearch) ([]Avatar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return fixtureSocialAvatars(a.records, search), nil
}
func fixtureSocialAvatars(records map[string]FixtureAccount, q SocialSearch) []Avatar {
	ids := map[string]bool{}
	for _, id := range q.OIDs {
		ids[id] = true
	}
	result := []Avatar{}
	for _, r := range records {
		for _, av := range r.Avatars {
			if len(ids) > 0 && !ids[hexOf(av.OID)] || q.UID > 0 && q.UID != av.UID || q.Hostnum > 0 && q.Hostnum != av.Hostnum || q.Nickname != "" && q.Nickname != av.Info.Nickname {
				continue
			}
			av.Progress = CloneProgress(av.Progress)
			result = append(result, av)
		}
	}
	sort.Slice(result, func(i, j int) bool { return hexOf(result[i].OID) < hexOf(result[j].OID) })
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
func (a *FixtureAccounts) UpdateSocial(ctx context.Context, ids []string, update func(map[string]*Avatar) error) ([]Avatar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	avatars := fixtureSocialAvatars(a.records, SocialSearch{OIDs: ids, Limit: 1000})
	found := map[string]*Avatar{}
	for i := range avatars {
		avatars[i].Progress.AvatarLevel = avatars[i].Info.Level
		found[hexOf(avatars[i].OID)] = &avatars[i]
	}
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			return nil, errors.New("社交角色不存在")
		}
	}
	if err := update(found); err != nil {
		return nil, err
	}
	for i := range avatars {
		avatars[i].Info.Level = avatars[i].Progress.AvatarLevel
	}
	for account, r := range a.records {
		for i, av := range r.Avatars {
			if latest, ok := found[hexOf(av.OID)]; ok {
				r.Avatars[i].Progress = CloneProgress(latest.Progress)
				r.Avatars[i].Info.Level = latest.Info.Level
			}
		}
		a.records[account] = r
	}
	return avatars, nil
}

// 夹具评论保存在角色社交状态，正式库使用独立带索引的card_comments表。
func (a *FixtureAccounts) QueryComments(ctx context.Context, card, start, order int) ([]CardComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := []CardComment{}
	for _, r := range a.records {
		for _, av := range r.Avatars {
			for _, c := range av.Progress.Social.Comments {
				if !c.Deleted && c.CardID == card {
					rows = append(rows, c)
				}
			}
		}
	}
	sortComments(rows, order)
	if start >= len(rows) {
		return []CardComment{}, nil
	}
	end := start + 20
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], nil
}
func (a *FixtureAccounts) UpdateComment(ctx context.Context, oid []byte, id string, fn func(*Avatar, *CardComment) error) (Avatar, CardComment, error) {
	if err := ctx.Err(); err != nil {
		return Avatar{}, CardComment{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	avatars := fixtureSocialAvatars(a.records, SocialSearch{Limit: 1000})
	var self *Avatar
	comment := CardComment{ID: id}
	for i := range avatars {
		av := &avatars[i]
		if string(av.OID) == string(oid) {
			self = av
		}
		if c, ok := av.Progress.Social.Comments[id]; ok {
			comment = c
		}
	}
	if self == nil {
		return Avatar{}, CardComment{}, errors.New("评论角色不存在")
	}
	self.Progress.AvatarLevel = self.Info.Level
	if err := fn(self, &comment); err != nil {
		return Avatar{}, CardComment{}, err
	}
	self.Info.Level = self.Progress.AvatarLevel
	owner := comment.Owner
	if owner == "" {
		return Avatar{}, CardComment{}, errors.New("评论归属缺失")
	}
	for account, r := range a.records {
		for i, av := range r.Avatars {
			if string(av.OID) == string(oid) {
				r.Avatars[i].Progress = CloneProgress(self.Progress)
				r.Avatars[i].Info.Level = self.Info.Level
			}
			if hexOf(av.OID) == owner {
				ensureSocial(&r.Avatars[i].Progress.Social)
				r.Avatars[i].Progress.Social.Comments[id] = comment
			}
		}
		a.records[account] = r
	}
	return *self, comment, nil
}
func SocialOID(id string) ([]byte, error) {
	if !validObjectID(id) || strings.ToLower(id) != id {
		return nil, errors.New("社交标识必须为十二字节")
	}
	b, err := hex.DecodeString(id)
	return b, err
}
func sortComments(rows []CardComment, order int) {
	sort.Slice(rows, func(i, j int) bool {
		if order == 2 && len(rows[i].Likes) != len(rows[j].Likes) {
			return len(rows[i].Likes) > len(rows[j].Likes)
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt > rows[j].CreatedAt
		}
		return rows[i].ID < rows[j].ID
	})
}
func cloneSocialState(s SocialState) SocialState {
	raw, _ := json.Marshal(s)
	var v SocialState
	_ = json.Unmarshal(raw, &v)
	return v
}
