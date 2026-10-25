package dbstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hs-server/internal/game"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSocialChangeEncodingParallelAndCancellation(t *testing.T) {
	avatars := map[string]*game.Avatar{}
	original := map[string][]byte{}
	ids := []string{}
	for index := 0; index < 32; index++ {
		id := fmt.Sprint(index)
		ids = append(ids, id)
		avatars[id] = &game.Avatar{Progress: game.Progress{PayCoin: int64(index), NewTasks: map[int]game.NewTaskProgress{101: {FinishedTargets: map[int]int{20102: index}}}}}
		original[id], _ = json.Marshal(avatars[id].Progress)
		if index%3 == 0 {
			avatars[id].Progress.NewTasks[101].FinishedTargets[20102]++
		}
	}
	result, err := marshalSocialChanges(context.Background(), avatars, ids, original, 4)
	if err != nil {
		t.Fatal(err)
	}
	for index := range ids {
		if index%3 != 0 {
			if result[index] != nil {
				t.Fatal("未变动进度被加入写入列表", index)
			}
			continue
		}
		var decoded game.Progress
		if err := json.Unmarshal(result[index], &decoded); err != nil || decoded.PayCoin != int64(index) || decoded.NewTasks[101].FinishedTargets[20102] != index+1 {
			t.Fatal("并行结果与角色顺序或嵌套变动不符", index, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := marshalSocialChanges(ctx, avatars, ids, original, 4); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("取消后仍返回可提交结果", err)
	}
	avatars["5"].Progress.CheckInRecords = map[int]any{1: make(chan int)}
	if result, err := marshalSocialChanges(context.Background(), avatars, ids, original, 4); err == nil || result != nil {
		t.Fatal("单个编码失败没有拒绝整批结果")
	}
}

func TestPostgresPeriodicParallelEncodingFailureRollsBack(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ids := []string{}
	oids := [][]byte{}
	balances := []int64{}
	for index := 0; index < 8; index++ {
		identity, err := s.Register(ctx, game.ClientInfo{Account: fmt.Sprintf("并行编码事务验收%d", index), Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		oid := identity.Avatars[0].OID
		ids = append(ids, hex.EncodeToString(oid))
		oids = append(oids, oid)
		balances = append(balances, pgHumanRead(t, s, oid).Progress.PayCoin)
	}
	calls := 0
	_, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		calls++
		for _, id := range ids {
			v[id].Progress.PayCoin += 17
		}
		if calls == 2 {
			v[ids[5]].Progress.CheckInRecords = map[int]any{1: make(chan int)}
		}
		return nil
	})
	if err == nil || calls != 2 {
		t.Fatal("未触发锁内编码失败", calls, err)
	}
	for index, oid := range oids {
		if pgHumanRead(t, s, oid).Progress.PayCoin != balances[index] {
			t.Fatal("编码失败导致其他角色部分入账", index)
		}
	}
}

type periodicQueryTrace struct {
	mu      sync.Mutex
	queries []string
}

func (p *periodicQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	p.mu.Lock()
	p.queries = append(p.queries, d.SQL)
	p.mu.Unlock()
	return ctx
}
func (*periodicQueryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresPeriodicPreloadPreservesConcurrentAndNestedState(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first, err := s.Register(ctx, game.ClientInfo{Account: "周期预读首个角色", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Register(ctx, game.ClientInfo{Account: "周期预读稳定角色", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := first.Avatars[0].OID
	id := hex.EncodeToString(oid)
	stableID := hex.EncodeToString(second.Avatars[0].OID)
	for _, av := range []game.Avatar{first.Avatars[0], second.Avatars[0]} {
		if _, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
			p.NewTasks = map[int]game.NewTaskProgress{101: {FinishedTargets: map[int]int{20102: 7}}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	trace := &periodicQueryTrace{}
	cfg := s.pool.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	traced := New(pool)
	var addedID string
	calls := 0
	rows, err := traced.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		calls++
		if calls == 1 {
			trace.mu.Lock()
			released := len(trace.queries) > 0 && strings.EqualFold(trace.queries[len(trace.queries)-1], "commit")
			trace.mu.Unlock()
			if !released {
				return errors.New("预览纯计算仍持有只读事务")
			}
			// 模拟维护写入未递增revision；xmin仍必须让锁内重读发生。
			if _, e := s.pool.Exec(ctx, `UPDATE avatar_progress SET state=jsonb_set(state,'{new_tasks,101,finished_targets,20102}','19'::jsonb) WHERE avatar_oid=$1`, oid); e != nil {
				return e
			}
			if _, e := s.pool.Exec(ctx, `UPDATE avatars SET level=12,nickname='并发修改后' WHERE avatar_oid=$1`, oid); e != nil {
				return e
			}
			created, e := s.Register(ctx, game.ClientInfo{Account: "周期预读后新增角色", Password: "pw", Hostnum: 1})
			if e != nil {
				return e
			}
			addedID = hex.EncodeToString(created.Avatars[0].OID)
		} else {
			if v[id].Progress.NewTasks[101].FinishedTargets[20102] != 19 || v[id].Info.Level != 12 || v[id].Info.Nickname != "并发修改后" {
				return errors.New("锁内没有重读并发进度或资料")
			}
			if v[stableID].Progress.NewTasks[101].FinishedTargets[20102] != 7 {
				return errors.New("原始快照被预览中的嵌套容器污染")
			}
			if v[addedID] == nil {
				return errors.New("预读后的新角色遗漏")
			}
		}
		for _, av := range v {
			if entry, ok := av.Progress.NewTasks[101]; ok {
				entry.FinishedTargets[20102]++
				av.Progress.NewTasks[101] = entry
			}
		}
		return nil
	})
	if err != nil || calls != 2 || len(rows) < 2 {
		t.Fatal("周期预读结算失败", err, calls, len(rows))
	}
	if pgHumanRead(t, s, oid).Progress.NewTasks[101].FinishedTargets[20102] != 20 || pgHumanRead(t, s, second.Avatars[0].OID).Progress.NewTasks[101].FinishedTargets[20102] != 8 {
		t.Fatal("周期预览重复入账或覆盖并发值")
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	versionReads, reloads := 0, 0
	for _, sql := range trace.queries {
		if strings.Contains(sql, "FOR UPDATE") && !strings.Contains(sql, "WHERE avatar_oid=ANY($1)") {
			t.Fatal("周期结算仍主动锁定全服", sql)
		}
		if strings.Contains(sql, "NULL::jsonb FROM avatar_progress ORDER BY avatar_oid") {
			versionReads++
		}
		if sql == `SELECT state FROM avatar_progress WHERE avatar_oid=$1` {
			reloads++
		}
	}
	if versionReads != 1 || reloads != 2 {
		t.Fatal("应仅重读并发变化角色和新增角色", versionReads, reloads)
	}
}

func TestPostgresPeriodicPreviewDoesNotBlockAndRecomputes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	identity, err := s.Register(ctx, game.ClientInfo{Account: "周期只读预览验收", Password: "pw", Hostnum: 1})
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	id := hex.EncodeToString(oid)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
			close(entered)
			<-release
			return nil
		})
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("预览未开始")
	}
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, err = s.UpdateProgress(short, oid, func(p *game.Progress) error { p.PayCoin += 7; return nil })
	cancel()
	close(release)
	if err != nil {
		t.Fatal("无变动周期预览阻塞玩家写入", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	before := pgHumanRead(t, s, oid).Progress.PayCoin
	calls := 0
	rows, err := s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		calls++
		if calls == 1 {
			if _, e := s.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.PayCoin += 11; return nil }); e != nil {
				return e
			}
		}
		v[id].Progress.PayCoin += 3
		return nil
	})
	if err != nil || calls != 2 || len(rows) != 1 {
		t.Fatal("有变动未重新加锁计算", err, calls, len(rows))
	}
	if got := pgHumanRead(t, s, oid).Progress.PayCoin; got != before+14 {
		t.Fatal("预览覆盖并发资产", got, before)
	}
	calls = 0
	_, err = s.UpdatePeriodicSocial(ctx, func(v map[string]*game.Avatar) error {
		calls++
		v[id].Progress.PayCoin += 99
		if calls == 2 {
			return errors.New("锁内故障注入")
		}
		return nil
	})
	if err == nil || pgHumanRead(t, s, oid).Progress.PayCoin != before+14 {
		t.Fatal("锁内失败没有整体回滚", err)
	}
}
