package game

import (
	"bytes"
	"context"
	"errors"
)

// SyncPvpRank 以角色ID消除同分歧义，hostnum=0查询世界榜；不受列表窗口限制。
func (a *FixtureAccounts) SyncPvpRank(ctx context.Context, oid []byte, hostnum int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var own *Avatar
	for _, record := range a.records {
		for _, av := range record.Avatars {
			if bytes.Equal(av.OID, oid) {
				v := av
				own = &v
			}
		}
	}
	if own == nil || (hostnum != 0 && own.Hostnum != hostnum) {
		return 0, errors.New("排行榜角色不存在")
	}
	rank := 1
	for _, record := range a.records {
		for _, av := range record.Avatars {
			if av.Progress.SyncPvpScore <= 0 || (hostnum != 0 && av.Hostnum != hostnum) {
				continue
			}
			if av.Progress.SyncPvpScore > own.Progress.SyncPvpScore || (av.Progress.SyncPvpScore == own.Progress.SyncPvpScore && bytes.Compare(av.OID, oid) < 0) {
				rank++
			}
		}
	}
	return rank, nil
}
