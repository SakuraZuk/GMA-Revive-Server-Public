package game

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type personalRankOnlyStore struct{ *FixtureAccounts }

func (a *personalRankOnlyStore) SyncPvpRankings(context.Context, int, ...int) ([]SyncPvpRankEntry, error) {
	return nil, errors.New("禁止为本人名次读取榜单")
}
func (a *personalRankOnlyStore) SocialAvatars(context.Context, SocialSearch) ([]Avatar, error) {
	return nil, errors.New("禁止为本人名次加载其他存档")
}

func TestSyncPvpPersonalRankAvoidsRosterAndFullProgress(t *testing.T) {
	store := &personalRankOnlyStore{NewFixtureAccounts(nil)}
	for i := 0; i < 1102; i++ {
		host := 2
		if i >= 1100 {
			host = 1
		}
		av := Avatar{OID: []byte(fmt.Sprintf("%012d", i)), Hostnum: host, Progress: Progress{SyncPvpScore: 1000}}
		store.records[fmt.Sprint(i)] = FixtureAccount{Avatars: []Avatar{av}}
	}
	av := store.records["1101"].Avatars[0]
	c := &Connection{phase: Playing, hostnum: 1, identity: Identity{Avatars: []Avatar{av}}}
	s := New(store, nil)
	for method, want := range map[string]int{"query_sync_pvp_rank": 2, "query_sync_pvp_world_rank": 1102} {
		out, err := s.Handle(context.Background(), c, method, socialArgs(77))
		if err != nil {
			t.Fatal(err)
		}
		values := socialCallback(t, out)
		if len(values) != 1 || values[0] != want {
			t.Fatal("本人名次被分页或跨服过滤改变", method, values)
		}
		if _, err := s.Handle(context.Background(), c, method, socialArgs(77, 1)); err == nil {
			t.Fatal("非法回调参数未拒绝")
		}
	}
}
