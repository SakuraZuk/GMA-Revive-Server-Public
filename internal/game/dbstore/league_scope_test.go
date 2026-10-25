package dbstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"hs-server/internal/game"
)

func TestPostgresLeagueScopedLocksProjectionAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	avs := []game.Avatar{}
	ids := []string{}
	for _, name := range []string{"学会锁本人", "学会锁主档", "学会锁目标", "学会锁无关"} {
		identity, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		av := identity.Avatars[0]
		avs = append(avs, av)
		ids = append(ids, hex.EncodeToString(av.OID))
	}
	leagueID := ids[1]
	_, err := s.UpdateSocial(ctx, ids[:3], func(v map[string]*game.Avatar) error {
		v[ids[0]].Progress.LeagueID = leagueID
		v[ids[1]].Progress.OwnedLeagues = map[string]*game.LeagueState{leagueID: {ID: leagueID, UID: 1, Host: 1, Members: map[string]game.LeagueMember{ids[0]: {Type: 1}}, Applications: map[string]int64{ids[2]: 1}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
	q := game.LeagueTransaction{SelfID: ids[0], TargetMember: ids[2]}
	// 无关角色同时锁住资料和进度，学会仍应完成。旧全服锁在这里必然超时。
	block, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(block)
	if _, err = block.Exec(ctx, `SELECT a.avatar_oid FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1 FOR UPDATE OF a,p`, avs[3].OID); err != nil {
		t.Fatal(err)
	}
	rows, err := traced.UpdateLeagueSocial(ctx, q, func(v map[string]*game.Avatar) error {
		if len(v[ids[3]].Progress.Cards) != 0 || len(v[ids[3]].Progress.Materials) != 0 {
			return errors.New("无关角色仍传输完整背包")
		}
		for _, id := range ids[:3] {
			if len(v[id].Progress.Cards) == 0 || v[id].Account == "" {
				return errors.New("相关角色完整存档或元数据丢失")
			}
		}
		v[ids[0]].Progress.LastLeaveLeague = 99
		v[ids[1]].Progress.OwnedLeagues[leagueID].PublicNotice = "已审批"
		v[ids[2]].Progress.LeagueID = leagueID
		return nil
	})
	if err != nil || len(rows) != 3 {
		t.Fatal("无关角色锁阻断学会或投影进入返回值", err, len(rows))
	}
	if err = block.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// 反向验证：学会回调持有自己的锁时，无关角色普通进度写入仍能完成。
	_, err = traced.UpdateLeagueSocial(ctx, q, func(v map[string]*game.Avatar) error {
		_, e := s.UpdateProgress(ctx, avs[3].OID, func(p *game.Progress) error { p.LastLeaveLeague = 777; return nil })
		return e
	})
	if err != nil {
		t.Fatal("学会锁阻断无关玩家普通写入", err)
	}
	for _, mode := range []string{"回调失败", "越界写入"} {
		before := []game.Progress{}
		for _, av := range avs {
			before = append(before, pgHumanRead(t, s, av.OID).Progress)
		}
		_, err = traced.UpdateLeagueSocial(ctx, q, func(v map[string]*game.Avatar) error {
			v[ids[0]].Progress.LastLeaveLeague++
			v[ids[1]].Progress.OwnedLeagues[leagueID].PublicNotice = "不应落盘"
			v[ids[2]].Progress.LeagueID = ""
			if mode == "回调失败" {
				return errors.New("审批故障注入")
			}
			v[ids[3]].Progress.LastLeaveLeague++
			return nil
		})
		if err == nil {
			t.Fatal("学会故障没有拒绝", mode)
		}
		for i, av := range avs {
			if !reflect.DeepEqual(before[i], pgHumanRead(t, s, av.OID).Progress) {
				t.Fatal("学会多行事务发生部分保存", mode, i)
			}
		}
	}
	// 查询不申请行锁、专属锁，不返回任何展示投影作为完整角色。
	trace.mu.Lock()
	trace.queries = nil
	trace.mu.Unlock()
	rows, err = traced.UpdateLeagueSocial(ctx, game.LeagueTransaction{SelfID: ids[0], ReadOnly: true}, func(v map[string]*game.Avatar) error {
		if v[ids[1]].Progress.OwnedLeagues[leagueID].PublicNotice != "已审批" || v[ids[2]].Progress.LeagueID != leagueID {
			return errors.New("学会只读快照丢失关系")
		}
		return nil
	})
	if err != nil || len(rows) != 0 {
		t.Fatal(err, len(rows))
	}
	trace.mu.Lock()
	queries := append([]string{}, trace.queries...)
	trace.mu.Unlock()
	for _, sql := range queries {
		if strings.Contains(strings.ToUpper(sql), "FOR UPDATE") || strings.Contains(sql, "pg_advisory") {
			t.Fatal("只读查询仍获取写锁", sql)
		}
	}
	if !strings.Contains(strings.Join(queries, "\n"), "jsonb_to_record") {
		t.Fatal("未执行学会字段投影")
	}
	trace.mu.Lock()
	trace.queries = nil
	trace.mu.Unlock()
	if _, err = traced.UpdateLeagueSocial(ctx, q, func(map[string]*game.Avatar) error { return nil }); err != nil {
		t.Fatal(err)
	}
	trace.mu.Lock()
	queries = append([]string{}, trace.queries...)
	trace.mu.Unlock()
	for _, sql := range queries {
		if strings.Contains(strings.ToUpper(sql), "FOR UPDATE") && !strings.Contains(sql, "avatar_oid=$1") {
			t.Fatal("学会仍获取全服行锁", sql)
		}
		if strings.Contains(sql, "p.state FROM") && !strings.Contains(sql, "avatar_oid=$1") {
			t.Fatal("学会仍传输全服完整存档", sql)
		}
	}
}

func TestPostgresLeagueConcurrentCreateUniqueNameUIDAndCost(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	type player struct {
		svc  *game.Service
		conn *game.Connection
		av   game.Avatar
	}
	players := []player{}
	for i := 0; i < 4; i++ {
		svc, conn, av := repairPlayer(t, s, game.ClientInfo{Account: fmt.Sprintf("并发学会%d", i), Password: "pw", Hostnum: 1}, &now)
		defer svc.Detach(conn)
		if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
			p.AvatarLevel = 6
			p.Materials[11] = game.Material{ID: 11, Count: 10000, Total: 10000}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		players = append(players, player{svc, conn, av})
	}
	type outcome struct {
		index int
		out   []game.Push
		err   error
	}
	run := func(indices []int, names []string) []outcome {
		start := make(chan struct{})
		done := make(chan outcome, len(indices))
		for j, i := range indices {
			go func(i int, name string) {
				<-start
				out, err := players[i].svc.Handle(ctx, players[i].conn, "create_new_league", pgSocialArgs(name, "并发验收", 4401))
				done <- outcome{i, out, err}
			}(i, names[j])
		}
		close(start)
		results := []outcome{}
		for range indices {
			results = append(results, <-done)
		}
		return results
	}
	success := 0
	for _, r := range run([]int{0, 1}, []string{"同名学会", "同名学会"}) {
		if r.err != nil {
			t.Fatal(r.err)
		}
		reply, _ := pgNativeReply(t, r.out, "on_create_new_league")
		p := pgHumanRead(t, s, players[r.index].av.OID).Progress
		if reply.Args[0] == game.RetSuccess {
			success++
			if p.LeagueID == "" || p.Materials[11].Count >= 10000 {
				t.Fatal("创建成功未扣费或未入会")
			}
		} else if p.LeagueID != "" || p.Materials[11].Count != 10000 {
			t.Fatal("同名拒绝仍扣费或入会")
		}
	}
	if success != 1 {
		t.Fatal("同名并发必须且只能一次成功", success)
	}
	for _, r := range run([]int{2, 3}, []string{"异名学会甲", "异名学会乙"}) {
		if r.err != nil {
			t.Fatal(r.err)
		}
		reply, _ := pgNativeReply(t, r.out, "on_create_new_league")
		if reply.Args[0] != game.RetSuccess {
			t.Fatal("异名创建失败", reply.Args)
		}
	}
	uids := map[int64]bool{}
	for _, player := range players {
		p := pgHumanRead(t, New(s.pool), player.av.OID).Progress
		for _, state := range p.OwnedLeagues {
			if state.UID <= 0 || uids[state.UID] {
				t.Fatal("并发学会编号重复", state.UID)
			}
			uids[state.UID] = true
		}
	}
	if len(uids) != 3 {
		t.Fatal("持久学会数量不正确", len(uids))
	}
	// 冷服务从真实学会主档构建推荐，类型必须能被原生league_info加载。
	fresh := pgSocialService(t, New(s.pool))
	cold := pgSocialLogin(t, fresh, game.ClientInfo{Account: "并发学会0", Password: "pw", Hostnum: 1}, players[0].av.OID)
	defer fresh.Detach(cold)
	out := pgSocialCall(t, fresh, cold, "get_recommend_leagues")
	recommendation, _ := pgNativeReply(t, out, "on_get_recommend_leagues")
	list := recommendation.Args[0].([]any)
	if len(list) != 3 {
		t.Fatal("冷推荐未返回三份真实学会", len(list))
	}
	for _, value := range list {
		info := value.(map[string]any)
		if info["trial_score"] != 0 || info["private_notice"] != "" {
			t.Fatal("冷推荐原生类型或私有公告错误")
		}
		for _, key := range []string{"boss_challenge_hps", "boss_max_hps"} {
			if _, ok := info[key].(map[int]int); !ok {
				t.Fatal("冷推荐血量类型不是字典", key)
			}
		}
	}
}
