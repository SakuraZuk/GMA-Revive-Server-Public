package game

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
)

// 周期结算须读全角色，不能把最多1000的推荐/搜索窗口当成全服排行。
func (a *FixtureAccounts) UpdateAllSocial(ctx context.Context, fn func(map[string]*Avatar) error) ([]Avatar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	avatars := map[string]*Avatar{}
	original := map[string][]byte{}
	for _, record := range a.records {
		for _, av := range record.Avatars {
			copy := av
			copy.Progress = CloneProgress(av.Progress)
			copy.Progress.AvatarLevel = av.Info.Level
			id := hexOf(av.OID)
			avatars[id] = &copy
			original[id], _ = json.Marshal(copy.Progress)
		}
	}
	if err := fn(avatars); err != nil {
		return nil, err
	}
	ids := []string{}
	for id, av := range avatars {
		raw, _ := json.Marshal(av.Progress)
		if !bytes.Equal(raw, original[id]) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result := []Avatar{}
	for _, id := range ids {
		av := avatars[id]
		av.Info.Level = av.Progress.AvatarLevel
		result = append(result, *av)
	}
	for account, record := range a.records {
		for i, av := range record.Avatars {
			latest := avatars[hexOf(av.OID)]
			record.Avatars[i].Progress = CloneProgress(latest.Progress)
			record.Avatars[i].Info.Level = latest.Progress.AvatarLevel
		}
		a.records[account] = record
	}
	return result, nil
}
