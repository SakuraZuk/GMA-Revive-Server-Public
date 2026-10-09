package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"hs-server/internal/game"
)

// 查询/双玩家/全服活动读回必须保留账号读取已确认的取名、性别、创建时间和等级。
func TestPostgresSocialMetadataReloadCalendarAndRealTutorialEntry(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "社交完整资料真实验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	named, err := store.SetNicknameGender(ctx, identity.Avatars[0].OID, "资料甲", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !named.NicknameSet || named.CreatedAt.IsZero() || named.Gender != 2 {
		t.Fatal("夹具不是实际已取名角色", named)
	}
	assertMetadata := func(av game.Avatar, level int) {
		t.Helper()
		if av.Account != info.Account || av.Hostnum != info.Hostnum || av.Info.Nickname != named.Info.Nickname ||
			!av.NicknameSet || av.Gender != named.Gender || !av.CreatedAt.Equal(named.CreatedAt) ||
			av.Info.Level != level || av.Info.HeadID != named.Info.HeadID || av.Info.HeadBoxID != named.Info.HeadBoxID ||
			av.Info.CustomHeadImageURL != named.Info.CustomHeadImageURL {
			t.Fatal("社交回载丢失官方账号角色元数据", av)
		}
	}
	id := hex.EncodeToString(named.OID)
	assertMetadata(pgHumanRead(t, New(store.pool), named.OID), named.Info.Level)
	rows, err := New(store.pool).UpdateSocial(ctx, []string{id}, func(v map[string]*game.Avatar) error {
		assertMetadata(*v[id], named.Info.Level)
		v[id].Progress.AvatarLevel = 9
		v[id].Progress.Social.Revision++
		return nil
	})
	if err != nil || len(rows) != 1 {
		t.Fatal(err, rows)
	}
	assertMetadata(rows[0], 9)
	rows, err = New(store.pool).UpdateAllSocial(ctx, func(v map[string]*game.Avatar) error {
		assertMetadata(*v[id], 9)
		v[id].Progress.Social.Revision++
		return nil
	})
	if err != nil || len(rows) != 1 {
		t.Fatal(err, rows)
	}
	assertMetadata(rows[0], 9)
	_, err = store.UpdateAllSocial(ctx, func(v map[string]*game.Avatar) error {
		v[id].Progress.AvatarLevel = 20
		return errors.New("社交全量资料进度故障注入")
	})
	if err == nil {
		t.Fatal("社交全量等级故障未回滚")
	}
	assertMetadata(pgHumanRead(t, New(store.pool), named.OID), 9)
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, New(store.pool))
	svc.Now = func() time.Time { return now }
	c := pgSocialLogin(t, svc, info, named.OID)
	selected, ok := c.SelectedAvatar()
	if !ok || selected.Progress.Activities.Calendar == nil {
		t.Fatal("实际BecomePlayer未经过活动日历回载")
	}
	assertMetadata(selected, 9)
	if selected.Progress.GuideTasks[1000].Status != 1 || selected.Progress.Battle != nil {
		t.Fatal("实际新账号必须仍处于1000创建引导，不能预设可入场")
	}
	type pgSnapshot struct {
		avatar, progress []byte
		revision         int64
		updated          time.Time
	}
	snapshot := func() pgSnapshot {
		t.Helper()
		var result pgSnapshot
		if err := store.pool.QueryRow(ctx, `SELECT to_jsonb(a),p.state,p.revision,p.updated_at FROM avatars a JOIN avatar_progress p ON p.avatar_oid=a.avatar_oid WHERE a.avatar_oid=$1`, named.OID).Scan(&result.avatar, &result.progress, &result.revision, &result.updated); err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeRejected := snapshot()
	if _, err := svc.Handle(ctx, c, "enter_dungeon", pgSocialArgs(8, 10001, map[string]any{})); err == nil || !strings.Contains(err.Error(), "引导期间的副本与当前引导不匹配") {
		t.Fatal("1000未完成时应按真实引导门槛拒绝10001", err)
	}
	afterRejected := snapshot()
	if !bytes.Equal(beforeRejected.avatar, afterRejected.avatar) || !bytes.Equal(beforeRejected.progress, afterRejected.progress) ||
		beforeRejected.revision != afterRejected.revision || !beforeRejected.updated.Equal(afterRejected.updated) {
		t.Fatal("引导拒绝入场后角色或完整PG进度发生写入")
	}
	// 原生先完成角色创建1000；后继1001视频运行时可预入其后继1002的10001。
	guidePushes, err := svc.Handle(ctx, c, "guide_task_finished", pgSocialArgs(71, 1000))
	if err != nil || len(guidePushes) != 1 || guidePushes[0].Method != "call_client_callback" || len(guidePushes[0].Args) != 2 || guidePushes[0].Args[0] != 71 {
		t.Fatal("真实guide_task_finished未回原生回调", err, guidePushes)
	}
	values, ok := guidePushes[0].Args[1].([]any)
	if !ok || len(values) != 2 || values[0] != true || values[1] != "" {
		t.Fatal("真实1000完成回调未成功", values)
	}
	guideAvatar := pgHumanRead(t, New(store.pool), named.OID)
	assertMetadata(guideAvatar, 9)
	if guideAvatar.Progress.GuideTasks[1000].Status != 2 || guideAvatar.Progress.GuideTasks[1001].Status != 1 ||
		guideAvatar.Progress.GuideTasks[1001].BeginLevel != 9 || guideAvatar.Progress.Battle != nil {
		t.Fatal("实际RPC未持久完成1000/激活1001，或提前创建战斗")
	}
	priorDay := selected.Progress.Activities.Calendar.NianDay
	now = now.Add(24 * time.Hour)
	// enter_dungeon本身再次触发全服活动结算回载，再进入真实10001创建引导。
	entryPushes, err := svc.Handle(ctx, c, "enter_dungeon", pgSocialArgs(9, 10001, map[string]any{}))
	if err != nil {
		t.Fatal("已取名角色经全服回载后不能进入原生10001", err)
	}
	selected, ok = c.SelectedAvatar()
	if !ok || selected.Progress.Battle == nil || selected.Progress.Battle.DungeonID != 10001 ||
		selected.Progress.Activities.Calendar == nil || selected.Progress.Activities.Calendar.NianDay == priorDay {
		t.Fatal("没有经过实际日历更新并持久创建10001会话")
	}
	assertMetadata(selected, 9)
	battle := selected.Progress.Battle
	if battle.UUID == "" || battle.Status != "准备" || battle.Loaded || battle.Started || battle.Finished || battle.LastSequence != 0 || battle.Bootstrap != "" {
		t.Fatal("10001未按原生创建新的冷启动会话", battle)
	}
	started := false
	for _, item := range entryPushes {
		if item.Method == "start_server_battle_ok" && len(item.Args) == 4 && item.Args[1] == 10001 && item.Args[2] == game.ObjectID(battle.UUID) {
			started = true
		}
	}
	if !started {
		t.Fatal("10001入场没有对应新UUID的原生启动回包")
	}
	reloaded, err := New(store.pool).QuickLogin(ctx, info)
	if err != nil || len(reloaded.Avatars) != 1 {
		t.Fatal(err)
	}
	assertMetadata(reloaded.Avatars[0], 9)
	if reloaded.Avatars[0].Progress.Battle == nil || reloaded.Avatars[0].Progress.Battle.UUID != selected.Progress.Battle.UUID {
		t.Fatal("实际进入10001后会话未随角色资料持久")
	}
	if reloaded.Avatars[0].Progress.GuideTasks[1000].Status != 2 || reloaded.Avatars[0].Progress.GuideTasks[1001].Status != 1 {
		t.Fatal("实际RPC引导状态未随冷启动会话重登保持")
	}
	// 明确零写断言覆盖整份PG进度，不只GUIDE/Battle片段。
	var rejectedState game.Progress
	if err := json.Unmarshal(afterRejected.progress, &rejectedState); err != nil || rejectedState.Battle != nil || rejectedState.GuideTasks[1000].Status != 1 {
		t.Fatal("拒绝快照没有保留原创建引导和空战斗", err)
	}
}

// 旧版本可能只写入avatars、未写avatar_progress；全服结算必须隔离该遗留行，
// 不能令有完整存档的新角色无法登录或进入教学副本。
func TestPostgresUpdateAllSocialSkipsLegacyAvatarWithoutProgress(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	legacyOID := game.NewAvatarOID(time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC))
	if _, err := store.pool.Exec(ctx, `INSERT INTO accounts(account,password_hash,created_at) VALUES($1,$2,now())`, "遗留孤立角色账号", "不可登录"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO avatars(avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head_image_url,account,gender,nickname_set,created_at) VALUES($1,900001,1,$2,1,1,3,'',$3,1,true,now())`, legacyOID, "遗留孤立角色", "遗留孤立角色账号"); err != nil {
		t.Fatal(err)
	}
	info := game.ClientInfo{Account: "孤立行隔离真实验收", Password: "pw", Hostnum: 1}
	identity, err := store.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(identity.Avatars[0].OID)
	rows, err := store.UpdateAllSocial(ctx, func(v map[string]*game.Avatar) error {
		if _, leaked := v[hex.EncodeToString(legacyOID)]; leaked {
			t.Fatal("无进度遗留角色进入了全服结算")
		}
		v[id].Progress.Social.Revision++
		return nil
	})
	if err != nil || len(rows) != 1 || !bytes.Equal(rows[0].OID, identity.Avatars[0].OID) {
		t.Fatal("遗留孤立角色阻断正常角色结算", rows, err)
	}
}
