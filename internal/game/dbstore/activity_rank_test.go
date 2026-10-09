package dbstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
)

func TestPostgresActivityRankingFullCountStableAndReload(t *testing.T) {
	// 默认未启用政策不能创建综合榜；此项仍验证原分榜全量SQL/分页与回滚。
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "")
	store := testStore(t)
	ctx := context.Background()
	batch := &pgx.Batch{}
	var own []byte
	for i := 1; i <= 1107; i++ {
		oid := make([]byte, 12)
		binary.BigEndian.PutUint32(oid[8:], uint32(i))
		account := fmt.Sprintf("活动排行%d", i)
		host := 10001
		d := game.ActivityProgress{ID: 20402011, Ranked: true, RankHard: 2, Actions: i, Cards: []any{[]any{4401, 20, 1, 440101, false}}}
		n := game.ActivityProgress{ID: 20200001, Ranked: true, RankDamage: int64(2000 - i), MaxDamage: 999999, RankCards: []any{[]any{4401, 20, 1, 440101, false}}}
		if i == 1106 {
			host = 10002
			d.RankHard = 999
			n.RankDamage = 9999999
		}
		if i == 1107 {
			d.Ranked = false
			n.Ranked = false
			d.RankHard = 999
			n.RankDamage = 9999999
		}
		p := game.Progress{AvatarLevel: 20, Activities: game.ActivityState{Mountain: &game.MountainState{Dungeons: map[int]game.ActivityProgress{1: d}}, Nian: map[int]game.ActivityProgress{20200001: n}, NianDay: "2026-10-08"}}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		batch.Queue(`INSERT INTO accounts(account,password_hash) VALUES($1,$2)`, account, "隔离夹具，不用于认证")
		batch.Queue(`INSERT INTO avatars(avatar_oid,account,hostnum,nickname,level) VALUES($1,$2,$3,$4,20)`, oid, account, host, account)
		batch.Queue(`INSERT INTO avatar_progress(avatar_oid,state) VALUES($1,$2::jsonb)`, oid, string(raw))
		if i == 1105 {
			own = oid
		}
	}
	if err := store.pool.SendBatch(ctx, batch).Close(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int{8, 10} {
		sub := 1
		if kind == 10 {
			sub = 20200001
		}
		rows, rank, err := store.ActivityRanking(ctx, kind, sub, own, 10001, 2)
		if err != nil || rank != 1105 || len(rows) != 2 {
			t.Fatal("SQL全量COUNT被页面窗口截断", kind, rank, len(rows), err)
		}
		if rows[0].Rank != 1 || rows[0].Avatar.Info.Nickname != "活动排行1" || len(rows[0].Cards) != 1 {
			t.Fatal("榜首/冻结阵容重载错误", rows[0])
		}
		if kind == 8 && (rows[0].Score[0] != 2 || rows[0].Score[1] != -1) {
			t.Fatal("原生负AP成绩错误", rows[0].Score)
		}
		if kind == 10 && rows[0].Score[0] != 1999 {
			t.Fatal("年兽使用个人max而不是有效榜成绩", rows[0].Score)
		}
	}
	// 同分稳定OID，另一服/旧桥无统计的成绩不得参榜。
	second := make([]byte, 12)
	binary.BigEndian.PutUint32(second[8:], 2)
	if _, err := store.UpdateProgress(ctx, second, func(p *game.Progress) error {
		d := p.Activities.Mountain.Dungeons[1]
		d.Actions = 1
		p.Activities.Mountain.Dungeons[1] = d
		n := p.Activities.Nian[20200001]
		n.RankDamage = 1999
		p.Activities.Nian[20200001] = n
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int{8, 10} {
		sub := 1
		if kind == 10 {
			sub = 20200001
		}
		rows, rank, err := store.ActivityRanking(ctx, kind, sub, second, 10001, 2)
		if err != nil || rank != 2 || rows[1].Avatar.Info.Nickname != "活动排行2" {
			t.Fatal("同分OID次序不一致", kind, rank, err)
		}
	}
	if _, _, err := store.ActivityRanking(ctx, 10, 0, own, 10001, 2); err == nil {
		t.Fatal("未知公式创建总榜")
	}
	// 新日界与有效成绩字段必须随玩家事务一并回滚，重建Store也保持原值。
	sentinel := errors.New("故障注入")
	if _, err := store.UpdateProgress(ctx, own, func(p *game.Progress) error {
		p.Activities.NianDay = "2026-10-09"
		d := p.Activities.Nian[20200001]
		d.RankDamage = 8888
		p.Activities.Nian[20200001] = d
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatal("活动故障未返回", err)
	}
	rows, rank, err := New(store.pool).ActivityRanking(ctx, 10, 20200001, own, 10001, 1000)
	if err != nil || rank != 1105 || len(rows) != 1000 {
		t.Fatal("重建Store未恢复全量名次", rank, len(rows), err)
	}
	var raw []byte
	if err = store.pool.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, own).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p game.Progress
	if err = json.Unmarshal(raw, &p); err != nil || p.Activities.NianDay != "2026-10-08" || p.Activities.Nian[20200001].RankDamage != 895 {
		t.Fatal("日界/有效成绩部分提交", err, p.Activities)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = store.ActivityRanking(canceled, 8, 1, own, 10001, 2); err == nil {
		t.Fatal("取消查询仍成功")
	}
}
