package dbstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
)

func remainingActivityCallback(t *testing.T, pushes []game.Push, method string, arity int) game.Push {
	t.Helper()
	var found game.Push
	count := 0
	for _, p := range pushes {
		if p.Method == method {
			found = p
			count++
		}
	}
	if count != 1 || len(found.Args) != arity {
		t.Fatal("原生回调缺失、重复、形状或ret错误", method, count, found)
	}
	raw, err := json.Marshal(found.Args[0])
	var status int
	if err != nil || json.Unmarshal(raw, &status) != nil || status != int(game.RetSuccess) {
		t.Fatal("原生回调ret不是成功枚举", method, found.Args[0])
	}
	return found
}

func remainingActivityJSONValue(value any) any {
	switch v := value.(type) {
	case mobileproto.Map:
		out := map[string]any{}
		for _, pair := range v {
			out[fmt.Sprint(pair.Key)] = remainingActivityJSONValue(pair.Value)
		}
		return out
	case map[int]int64:
		out := map[string]any{}
		for k, x := range v {
			out[fmt.Sprint(k)] = x
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			out[k] = remainingActivityJSONValue(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = remainingActivityJSONValue(x)
		}
		return out
	default:
		return value
	}
}

func remainingActivitySameBox(t *testing.T, wire any, saved map[string]any) {
	t.Helper()
	a, err := json.Marshal(remainingActivityJSONValue(wire))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || !bytes.Equal(a, b) {
		t.Fatal("回调奖励盒不是原冻结收据", string(a), string(b))
	}
}

func remainingCthulhuCallbackReceipt(t *testing.T, pushes []game.Push, p game.Progress) {
	t.Helper()
	reply := remainingActivityCallback(t, pushes, "on_cthulhu_item_check", 3)
	m := p.Activities.Cthulhu.Maps[1]
	r := m.Trunks[0].Item.CheckReceipt
	if r == nil {
		t.Fatal("成功检定回调缺持久收据")
	}
	check := 2
	if r.Success {
		check = 1
	}
	expected := map[string]any{"dice1": r.Dice[0], "dice2": r.Dice[1], "check_res": check, "big_enable": r.Big}
	a, _ := json.Marshal(reply.Args[1])
	b, _ := json.Marshal(expected)
	if !bytes.Equal(a, b) {
		t.Fatal("回调骰值/成功分支不是原冻结结果", string(a), string(b))
	}
	remainingActivitySameBox(t, reply.Args[2], r.Box)
}

func TestPostgresRemainingActivityFrozenTotalBeyondPageAndRollback(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	batch := &pgx.Batch{}
	var own []byte
	for i := 1; i <= 1106; i++ {
		oid := make([]byte, 12)
		binary.BigEndian.PutUint32(oid[8:], uint32(i))
		host := 10001
		if i == 1106 {
			host = 10002
		}
		p := game.Progress{Activities: game.ActivityState{Calendar: &game.ActivityCalendar{NianTotal: &game.NianTotalSnapshot{Hour: 1800000000, Rank: i, Score: int64(i + 2), Joined: 1, SubRanks: [3]int{i, 1, 1}, Cards: []any{[]any{4401, 20}}}}}}
		raw, _ := json.Marshal(p)
		account := fmt.Sprintf("本服总榜%d", i)
		batch.Queue(`INSERT INTO accounts(account,password_hash) VALUES($1,$2)`, account, "隔离夹具")
		batch.Queue(`INSERT INTO avatars(avatar_oid,account,hostnum,nickname,level) VALUES($1,$2,$3,$4,20)`, oid, account, host, account)
		batch.Queue(`INSERT INTO avatar_progress(avatar_oid,state) VALUES($1,$2::jsonb)`, oid, string(raw))
		if i == 1105 {
			own = oid
		}
	}
	if err := store.pool.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	rows, rank, err := New(store.pool).ActivityRanking(ctx, 10, 0, own, 10001, 2)
	if err != nil || rank != 1105 || len(rows) != 2 || rows[1].SubRanks != [3]int{2, 1, 1} {
		t.Fatal("JSONB冻结总榜全量个人名次/分页/服隔离", rank, len(rows), err)
	}
}

func TestPostgresRemainingActivityWeeklyTotalMailAtomicAndRetry(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	zone := time.FixedZone("北京时间", 28800)
	begin := time.Date(2026, 10, 7, 0, 0, 0, 0, zone)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(begin)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, zone)
	svc := pgActivityService(t, store, func() time.Time { return now })
	oids := [][]byte{}
	cs := []*game.Connection{}
	for i := 1; i <= 2; i++ {
		info := game.ClientInfo{Account: fmt.Sprintf("本服周奖%d", i), Password: "pw", Hostnum: 10001}
		identity, err := store.Register(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		oid := identity.Avatars[0].OID
		if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error { p.AvatarLevel = 20; p.ClearedDungeons = []int{601, 610}; return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `UPDATE avatars SET level=20 WHERE avatar_oid=$1`, oid); err != nil {
			t.Fatal(err)
		}
		cs = append(cs, pgActivityLogin(t, svc, info))
		oids = append(oids, oid)
	}
	scoreAt := time.Date(2026, 10, 20, 12, 0, 0, 0, zone).Unix()
	for i, oid := range oids {
		if _, err := store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
			p.Activities.Nian = map[int]game.ActivityProgress{20200001: {Ranked: true, RankDamage: int64(100 - i*10), RankAt: scoreAt, MaxDamage: 999}}
			p.Activities.NianHistory = map[int]map[int64]game.NianScoreSnapshot{20200001: {scoreAt / 3600: {At: scoreAt, Damage: int64(100 - i*10)}}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpdateProgress(ctx, oids[1], func(p *game.Progress) error {
		p.ShortMailInfo = map[int]game.Mail{}
		for i := 1; i <= 500; i++ {
			p.ShortMailInfo[i] = game.Mail{MID: i, UUID: fmt.Sprintf("%024x", 10000+i), Title: "容量夹具"}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := [][]byte{}
	for _, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		before = append(before, raw)
	}
	now = time.Date(2026, 10, 21, 23, 59, 59, 0, zone)
	if _, err := svc.Handle(ctx, cs[0], "query_rank_list", pgSocialArgs(10, 0)); err == nil {
		t.Fatal("周奖容量故障未阻止全服事务")
	}
	for i, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
		if !bytes.Equal(before[i], raw) {
			t.Fatal("周榜清理/奖邮件/收据部分提交", i)
		}
	}
	if _, err := store.UpdateProgress(ctx, oids[1], func(p *game.Progress) error { p.ShortMailInfo = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Handle(ctx, cs[0], "query_rank_list", pgSocialArgs(10, 0)); err != nil {
		t.Fatal(err)
	}
	after := [][]byte{}
	for i, oid := range oids {
		p := pgHumanRead(t, store, oid).Progress
		r := p.Activities.Calendar.NianWeeks["2026-10-21"]
		if r.TotalRank != i+1 || !r.Issued || r.Bonus == 0 || r.Pending != "" || p.Activities.Nian[20200001].RankDamage != 0 || p.Activities.Nian[20200001].MaxDamage != 999 {
			t.Fatal("本服周奖/清榜/保留个人纪录", r)
		}
		raw, _ := json.Marshal(p)
		after = append(after, raw)
	}
	if _, err := svc.Handle(ctx, cs[0], "query_rank_list", pgSocialArgs(10, 0)); err != nil {
		t.Fatal(err)
	}
	for i, oid := range oids {
		raw, _ := json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
		if !bytes.Equal(after[i], raw) {
			t.Fatal("重建Store周奖重试重复邮件", i)
		}
	}
}

func TestPostgresRemainingActivityCthulhuAndSurpriseReceiptReload(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	zone := time.FixedZone("北京时间", 28800)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, zone)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(time.Date(2026, 10, 7, 0, 0, 0, 0, zone))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	info := game.ClientInfo{Account: "本服检定惊喜", Password: "pw", Hostnum: 10001}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.AvatarLevel = 40
		p.ClearedDungeons = []int{601, 610}
		p.Materials[53] = game.Material{Count: 100, Total: 100}
		p.Materials[54] = game.Material{Count: 6, Total: 6}
		p.Activities.Cthulhu = &game.CthulhuState{Maps: map[int]*game.CthulhuMap{1: {ID: 1, Layer: 1010, Layers: map[int]int{1010: 1}, Trunks: map[int]*game.CthulhuGrid{0: {Item: &game.CthulhuItem{ID: 1201, Index: -1, Status: 1}}}}}}
		p.Activities.Miku = &game.MikuState{Maps: map[int]*game.MikuMap{1: {ID: 1, CompletedRuns: 1}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, `UPDATE avatars SET level=40 WHERE avatar_oid=$1`, oid); err != nil {
		t.Fatal(err)
	}
	svc := pgActivityService(t, store, func() time.Time { return now })
	c := pgActivityLogin(t, svc, info)
	pushes, err := svc.Handle(ctx, c, "cthulhu_item_check", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal("真实检定RPC失败", err, pushes)
	}
	remainingCthulhuCallbackReceipt(t, pushes, pgHumanRead(t, store, oid).Progress)
	before, _ := json.Marshal(pgHumanRead(t, store, oid).Progress)
	if pushes, err = svc.Handle(ctx, c, "cthulhu_item_check", pgSocialArgs(1201, 1)); err != nil {
		t.Fatal(err)
	}
	remainingCthulhuCallbackReceipt(t, pushes, pgHumanRead(t, store, oid).Progress)
	after, _ := json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实JSONB检定重复掷骰/重复奖")
	}
	// 独立失败快照用于确定覆盖all-in成本、结果冻结及JSONB重载，不依赖随机掷出失败。
	if _, err = store.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Materials[53] = game.Material{Count: 100, Total: 100}
		m := p.Activities.Cthulhu.Maps[1]
		x := m.Trunks[0].Item
		x.Status = 2
		x.CheckReceipt = &game.CthulhuCheckReceipt{Visit: m.Moves, Cycle: m.Cycle, Layer: m.Layer, Trunk: m.Trunk, Branch: m.Branch, Dice: [2]int{6, 6}, Big: true, Success: false}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err = svc.Handle(ctx, c, "cthulhu_check_all_in", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal("真实all-in RPC失败", err, pushes)
	}
	p := pgHumanRead(t, store, oid).Progress
	remainingCthulhuCallbackReceipt(t, pushes, p)
	r := p.Activities.Cthulhu.Maps[1].Trunks[0].Item.CheckReceipt
	expected := int64(80)
	if r.Big {
		if r.Success {
			expected += 20
		} else {
			expected -= 10
		}
	}
	if !r.AllIn || p.Materials[53].Count != expected {
		t.Fatal("真实all-in成本/SAN", r, p.Materials[53])
	}
	before, _ = json.Marshal(p)
	if pushes, err = svc.Handle(ctx, c, "cthulhu_check_all_in", pgSocialArgs(1201, 1)); err != nil {
		t.Fatal(err)
	}
	remainingCthulhuCallbackReceipt(t, pushes, pgHumanRead(t, store, oid).Progress)
	after, _ = json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("JSONB all-in重试重复成本")
	}
	if pushes, err = svc.Handle(ctx, c, "receive_miku_surprise_bonus", pgSocialArgs(1)); err != nil {
		t.Fatal(err)
	}
	p = pgHumanRead(t, store, oid).Progress
	reply := remainingActivityCallback(t, pushes, "on_receive_miku_surprise_bonus", 2)
	remainingActivitySameBox(t, reply.Args[1], p.Activities.Miku.Maps[1].SurpriseBox)
	if p.Materials[208095].Count != 10 || !p.Activities.Miku.Maps[1].SurpriseClaimed {
		t.Fatal("真实惊喜资产与一次收据未同提交")
	}
	before, _ = json.Marshal(p)
	if pushes, err = svc.Handle(ctx, c, "receive_miku_surprise_bonus", pgSocialArgs(1)); err != nil {
		t.Fatal(err)
	}
	reply = remainingActivityCallback(t, pushes, "on_receive_miku_surprise_bonus", 2)
	remainingActivitySameBox(t, reply.Args[1], pgHumanRead(t, store, oid).Progress.Activities.Miku.Maps[1].SurpriseBox)
	after, _ = json.Marshal(pgHumanRead(t, New(store.pool), oid).Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("JSONB惊喜重试重复领")
	}
}

func TestPostgresRemainingActivityZeroRankReceiptAndUnknownAbsent(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	store := testStore(t)
	ctx := context.Background()
	oids := [][]byte{}
	for i := 1; i <= 3; i++ {
		oid := make([]byte, 12)
		binary.BigEndian.PutUint32(oid[8:], uint32(i))
		oids = append(oids, oid)
		d := game.ActivityProgress{Ranked: true, RankAt: 1800000000}
		if i == 1 {
			d.RankDamage = 100
		}
		if i == 3 {
			d.RankAt = 0
		}
		p := game.Progress{Activities: game.ActivityState{Nian: map[int]game.ActivityProgress{20200001: d}}}
		raw, _ := json.Marshal(p)
		account := fmt.Sprintf("零伤害参赛%d", i)
		if _, err := store.pool.Exec(ctx, `INSERT INTO accounts(account,password_hash) VALUES($1,$2)`, account, "隔离夹具"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `INSERT INTO avatars(avatar_oid,account,hostnum,nickname,level) VALUES($1,$2,10001,$3,20)`, oid, account, account); err != nil {
			t.Fatal(err)
		}
		if _, err := store.pool.Exec(ctx, `INSERT INTO avatar_progress(avatar_oid,state) VALUES($1,$2::jsonb)`, oid, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	rows, rank, err := New(store.pool).ActivityRanking(ctx, 10, 20200001, oids[1], 10001, 2)
	if err != nil || rank != 2 || len(rows) != 2 || rows[0].Score[0] != 100 || rows[1].Score[0] != 0 {
		t.Fatal("SQL/JSONB真实零分不参榜或正分顺序改变", rank, len(rows), err)
	}
	_, rank, err = store.ActivityRanking(ctx, 10, 20200001, oids[2], 10001, 2)
	if err != nil || rank != 0 {
		t.Fatal("SQL旧未知零字段补造参与", rank, err)
	}
}

// 可执行本地真实Handle合同，数据库专项仍使用其真实Store；不假定属性推送与回调的末项顺序。
func TestRemainingActivityNativeCallbacksFromFixtureActualHandle(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	ctx := context.Background()
	zone := time.FixedZone("北京时间", 28800)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, zone)
	if err := game.SetActivitySchedules(game.DefaultPermanentActivitySchedules(time.Date(2026, 10, 7, 0, 0, 0, 0, zone))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = game.SetActivitySchedules(nil) })
	accounts := game.NewFixtureAccounts(nil)
	info := game.ClientInfo{Account: "活动回调方法真实合同", Password: "pw", Hostnum: 10001}
	identity, err := accounts.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	oid := identity.Avatars[0].OID
	id := fmt.Sprintf("%x", oid)
	if _, err = accounts.UpdateSocial(ctx, []string{id}, func(rows map[string]*game.Avatar) error {
		p := &rows[id].Progress
		p.AvatarLevel = 40
		p.ClearedDungeons = []int{601, 610}
		p.Materials[53] = game.Material{Count: 100, Total: 100}
		p.Materials[54] = game.Material{Count: 6, Total: 6}
		p.Activities.Cthulhu = &game.CthulhuState{Maps: map[int]*game.CthulhuMap{1: {ID: 1, Layer: 1010, Layers: map[int]int{1010: 1}, Trunks: map[int]*game.CthulhuGrid{0: {Item: &game.CthulhuItem{ID: 1201, Index: -1, Status: 1}}}}}}
		p.Activities.Miku = &game.MikuState{Maps: map[int]*game.MikuMap{1: {ID: 1, CompletedRuns: 1}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var catalog hotfix.Catalog
	if err = catalog.Load(filepath.Join("..", "..", "..", "deploy", "data", "hotfix.json")); err != nil {
		t.Fatal(err)
	}
	svc := game.New(accounts, &catalog)
	svc.Now = func() time.Time { return now }
	c := game.NewConnection()
	c.SetDeviceID("回调方法夹具设备")
	pushes, err := svc.Handle(ctx, c, "quick_login", pgSocialArgs(info))
	if err != nil || len(pushes) != 3 || pushes[0].Args[0] != game.RetSuccess {
		t.Fatal("真实夹具登录失败", err, pushes)
	}
	if _, err = svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	defer svc.Detach(c)
	if _, err = svc.Handle(ctx, c, "set_reconnect_auth_msg", pgSocialArgs("真实夹具绑定")); err != nil {
		t.Fatal(err)
	}
	pushes, err = svc.Handle(ctx, c, "cthulhu_item_check", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal(err)
	}
	remainingCthulhuCallbackReceipt(t, pushes, c.SelectedAvatarUnsafe().Progress)
	// 明确允许任意合法属性推送后置，防止“最后一项就是回调”的旧断言回归。
	pushes = append(pushes, game.Push{Method: "client_prop_changed", Args: []any{[]any{"exp", 1000}}})
	remainingCthulhuCallbackReceipt(t, pushes, c.SelectedAvatarUnsafe().Progress)
	before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	pushes, err = svc.Handle(ctx, c, "cthulhu_item_check", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal(err)
	}
	remainingCthulhuCallbackReceipt(t, pushes, c.SelectedAvatarUnsafe().Progress)
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实夹具首次重试重复资产或收据")
	}
	if _, err = accounts.UpdateProgress(ctx, oid, func(p *game.Progress) error {
		p.Materials[53] = game.Material{Count: 100, Total: 100}
		m := p.Activities.Cthulhu.Maps[1]
		x := m.Trunks[0].Item
		x.Status = 2
		x.CheckReceipt = &game.CthulhuCheckReceipt{Visit: m.Moves, Cycle: m.Cycle, Layer: m.Layer, Trunk: m.Trunk, Branch: m.Branch, Dice: [2]int{6, 6}, Big: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pushes, err = svc.Handle(ctx, c, "cthulhu_check_all_in", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	remainingCthulhuCallbackReceipt(t, pushes, p)
	r := p.Activities.Cthulhu.Maps[1].Trunks[0].Item.CheckReceipt
	expected := int64(80)
	if r.Big {
		if r.Success {
			expected += 20
		} else {
			expected -= 10
		}
	}
	if !r.AllIn || p.Materials[53].Count != expected {
		t.Fatal("真实夹具重掷成本或SAN错误")
	}
	before, _ = json.Marshal(p)
	pushes, err = svc.Handle(ctx, c, "cthulhu_check_all_in", pgSocialArgs(1201, 1))
	if err != nil {
		t.Fatal(err)
	}
	remainingCthulhuCallbackReceipt(t, pushes, c.SelectedAvatarUnsafe().Progress)
	after, _ = json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实夹具重掷重试重复资产或收据")
	}
	pushes, err = svc.Handle(ctx, c, "receive_miku_surprise_bonus", pgSocialArgs(1))
	if err != nil {
		t.Fatal(err)
	}
	p = c.SelectedAvatarUnsafe().Progress
	reply := remainingActivityCallback(t, pushes, "on_receive_miku_surprise_bonus", 2)
	remainingActivitySameBox(t, reply.Args[1], p.Activities.Miku.Maps[1].SurpriseBox)
	if p.Materials[208095].Count != 10 || !p.Activities.Miku.Maps[1].SurpriseClaimed {
		t.Fatal("真实夹具惊喜资产/收据不匹配")
	}
	before, _ = json.Marshal(p)
	pushes, err = svc.Handle(ctx, c, "receive_miku_surprise_bonus", pgSocialArgs(1))
	if err != nil {
		t.Fatal(err)
	}
	reply = remainingActivityCallback(t, pushes, "on_receive_miku_surprise_bonus", 2)
	remainingActivitySameBox(t, reply.Args[1], c.SelectedAvatarUnsafe().Progress.Activities.Miku.Maps[1].SurpriseBox)
	after, _ = json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("真实夹具惊喜重试重复资产或收据")
	}
	// 本版原生回包命名与上行不同，必须检查实际Handle输出而不是拼接方法名。
	coinsBefore := c.SelectedAvatarUnsafe().Progress.Materials[12].Count
	pushes, err = svc.Handle(ctx, c, "receive_miku_achv_bonus", pgSocialArgs(1101))
	if err != nil {
		t.Fatal(err)
	}
	reply = remainingActivityCallback(t, pushes, "on_receive_miku_achv_reward", 2)
	if c.SelectedAvatarUnsafe().Progress.Materials[12].Count != coinsBefore+20000 || !c.SelectedAvatarUnsafe().Progress.Activities.Miku.Achievements[1101].Claimed {
		t.Fatal("初音1101原生成就回包、奖励或领取状态不匹配", reply, "领取前金币", coinsBefore, "领取后金币", c.SelectedAvatarUnsafe().Progress.Materials[12].Count, "领取状态", c.SelectedAvatarUnsafe().Progress.Activities.Miku.Achievements[1101])
	}
	before, _ = json.Marshal(c.SelectedAvatarUnsafe().Progress)
	pushes, err = svc.Handle(ctx, c, "receive_miku_achv_bonus", pgSocialArgs(1101))
	if err != nil {
		t.Fatal(err)
	}
	failure := pgRemainingPush(pushes, "on_receive_miku_achv_reward")
	after, _ = json.Marshal(c.SelectedAvatarUnsafe().Progress)
	var status int
	if failure == nil || len(failure.Args) != 2 {
		t.Fatal("初音重复领取缺完整原生失败回包", pushes)
	}
	statusJSON, statusErr := json.Marshal(failure.Args[0])
	if statusErr != nil || json.Unmarshal(statusJSON, &status) != nil || status == int(game.RetSuccess) || !bytes.Equal(before, after) {
		t.Fatal("初音重复领取必须完整失败回包且资产/收据不变", failure)
	}
}
