package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"hs-server/internal/game"
)

func periodicSerializationFailure(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "40001"
}

func TestPostgresPeriodicWritePhaseAllowsPeerAndRejectsStaleTarget(t *testing.T) {
	for _, sameTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "未变动角色不阻塞", true: "同角色冲突整轮回滚"}[sameTarget], func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			var avatars []game.Avatar
			for _, name := range []string{"周期获奖者", "正常写入者"} {
				identity, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
				if err != nil {
					t.Fatal(err)
				}
				avatars = append(avatars, identity.Avatars[0])
			}
			target := hex.EncodeToString(avatars[0].OID)
			base := pgHumanRead(t, s, avatars[0].OID).Progress.PayCoin
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				calls := 0
				_, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
					calls++
					v[target].Progress.PayCoin += 3
					if calls == 2 {
						close(entered)
						<-release
					}
					return nil
				})
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				close(release)
				t.Fatal("周期写阶段未开始")
			}
			peer := avatars[1]
			if sameTarget {
				peer = avatars[0]
			}
			short, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err := s.UpdateProgress(short, peer.OID, func(p *game.Progress) error { p.PayCoin += 11; return nil })
			cancel()
			close(release)
			periodicErr := <-done
			if err != nil {
				t.Fatal("周期纯计算仍阻塞玩家写入", err)
			}
			if sameTarget {
				if !periodicSerializationFailure(periodicErr) || pgHumanRead(t, s, avatars[0].OID).Progress.PayCoin != base+11 {
					t.Fatal("旧快照未拒绝或覆盖了并发资产", periodicErr)
				}
				_, err = s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error { v[target].Progress.PayCoin += 3; return nil })
				if err != nil || pgHumanRead(t, s, avatars[0].OID).Progress.PayCoin != base+14 {
					t.Fatal("新快照重试未且仅提交一次", err)
				}
			} else if periodicErr != nil || pgHumanRead(t, s, avatars[0].OID).Progress.PayCoin != base+3 {
				t.Fatal("无关写入损坏结算", periodicErr)
			}
		})
	}
}

// 周期写阶段数据库级单一执行者，第二轮不得等待旧快照后提交。
func TestPostgresConcurrentPeriodicRoundsRejectWriteSkew(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	identity, err := s.Register(ctx, game.ClientInfo{Account: "周期独占", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	id := hex.EncodeToString(oid)
	base := pgHumanRead(t, s, oid).Progress.PayCoin
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		calls := 0
		_, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
			calls++
			v[id].Progress.PayCoin++
			if calls == 2 {
				close(entered)
				<-release
			}
			return nil
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("周期写阶段未开始")
	}
	_, err = s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error { v[id].Progress.PayCoin++; return nil })
	close(release)
	firstErr := <-done
	if !errors.Is(err, errPeriodicDependencyBusy) || firstErr != nil || pgHumanRead(t, s, oid).Progress.PayCoin != base+1 {
		t.Fatal("并发周期没有拒绝第二个写入者", firstErr, err)
	}
	// 同一连接池下一轮可以成功，保护锁不得在提交后泄漏。
	_, err = s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error { v[id].Progress.PayCoin++; return nil })
	if err != nil || pgHumanRead(t, s, oid).Progress.PayCoin != base+2 {
		t.Fatal("周期保护锁提交后未释放", err)
	}
}

// 普通双角色事务也参与周期的跨行依赖，不能只验证两个可串行化周期之间的冲突。
func TestPostgresPeriodicRejectsMixedSocialWriteSkew(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := []string{}
	oids := [][]byte{}
	for _, name := range []string{"混合依赖甲", "混合依赖乙"} {
		identity, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		oid := identity.Avatars[0].OID
		if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.PayCoin = 0; return nil }); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, hex.EncodeToString(oid))
		oids = append(oids, oid)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		calls := 0
		_, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
			calls++
			if v[ids[1]].Progress.PayCoin == 0 {
				v[ids[0]].Progress.PayCoin = 1
			}
			if calls == 2 {
				close(entered)
				<-release
			}
			return nil
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("混合事务周期阶段未开始")
	}
	_, socialErr := s.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		if v[ids[0]].Progress.PayCoin == 0 {
			v[ids[1]].Progress.PayCoin = 1
		}
		return nil
	})
	close(release)
	periodicErr := <-done
	var lockError *pgconn.PgError
	if !errors.As(socialErr, &lockError) || lockError.Code != "55P03" || periodicErr != nil {
		t.Fatal("跨角色写入没有在读取旧依赖前被拒绝", socialErr, periodicErr)
	}
	if pgHumanRead(t, s, oids[0]).Progress.PayCoin != 1 || pgHumanRead(t, s, oids[1]).Progress.PayCoin != 0 {
		t.Fatal("混合事务仍形成写偏差")
	}
	// 保护锁释放后，社交重试必须读取已经提交的甲，不能按旧值给乙加钱。
	_, err := s.UpdateSocial(ctx, ids, func(v map[string]*game.Avatar) error {
		if v[ids[0]].Progress.PayCoin == 0 {
			v[ids[1]].Progress.PayCoin = 1
		}
		return nil
	})
	if err != nil || pgHumanRead(t, s, oids[1]).Progress.PayCoin != 0 {
		t.Fatal("社交重试读到过期依赖", err)
	}
}

// 反向顺序：已有跨角色事务时，周期不能先取旧快照再等待其结束。
func TestPostgresPeriodicDefersToActiveCrossRoleTransaction(t *testing.T) {
	for _, mode := range []string{"普通社交", "全服管理", "学会写入"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			ids := []string{}
			oids := [][]byte{}
			for _, name := range []string{"依赖前置甲", "依赖前置乙"} {
				u, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
				if err != nil {
					t.Fatal(err)
				}
				oid := u.Avatars[0].OID
				if _, err = s.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.PayCoin = 0; return nil }); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, hex.EncodeToString(oid))
				oids = append(oids, oid)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				callback := func(v map[string]*game.Avatar) error {
					if v[ids[0]].Progress.PayCoin == 0 {
						v[ids[1]].Progress.PayCoin = 1
					}
					close(entered)
					<-release
					return nil
				}
				var err error
				switch mode {
				case "普通社交":
					_, err = s.UpdateSocial(ctx, ids, callback)
				case "全服管理":
					_, err = s.UpdateAllSocial(ctx, callback)
				case "学会写入":
					_, err = s.UpdateLeagueSocial(ctx, game.LeagueTransaction{SelfID: ids[0], TargetMember: ids[1]}, callback)
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				close(release)
				t.Fatal("跨角色事务未开始")
			}
			callback := func(v map[string]*game.Avatar) error {
				if v[ids[1]].Progress.PayCoin == 0 {
					v[ids[0]].Progress.PayCoin = 1
				}
				return nil
			}
			_, err := s.UpdatePeriodicSocial(ctx, callback)
			close(release)
			peerErr := <-done
			if !errors.Is(err, errPeriodicDependencyBusy) || peerErr != nil {
				t.Fatal("周期没有避让已有跨角色事务", err, peerErr)
			}
			_, err = s.UpdatePeriodicSocial(ctx, callback)
			if err != nil || pgHumanRead(t, s, oids[0]).Progress.PayCoin != 0 || pgHumanRead(t, s, oids[1]).Progress.PayCoin != 1 {
				t.Fatal("周期重试仍使用旧依赖", err)
			}
		})
	}
}
