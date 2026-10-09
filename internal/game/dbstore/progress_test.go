package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/hotfix"
)

func TestPostgresAdminRuneGrantAtomicAudit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "管理发契印验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av := id.Avatars[0]
	service := game.New(s, nil)
	token := strings.Repeat("测试令牌", 16)
	handler := game.AdminHandler(service, token)
	body, _ := json.Marshal(map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "数据库并发验收", "operator": "测试管理员", "reason": "独立测试 schema 验收", "spec": game.RuneSpec{Suit: 1101, Position: 1, Star: 5, Level: 1}})
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodPost, "/admin/runes/grant", bytes.NewReader(body))
			request.RemoteAddr = "127.0.0.1:12345"
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			responses <- response
		}()
	}
	wg.Wait()
	close(responses)
	created := 0
	for response := range responses {
		if response.Code == http.StatusCreated {
			created++
		} else if response.Code != http.StatusOK {
			t.Fatalf("并发发放失败: %d %s", response.Code, response.Body.String())
		}
	}
	if created != 1 {
		t.Fatal("真实数据库并发请求未保证一次发放")
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if len(p.Runes) != 1 || len(p.RuneGrantReceipts) != 1 || len(p.RuneAdminAudit) != 1 {
		t.Fatal("奖励、凭据、审计未完整持久化")
	}
	audit := p.RuneAdminAudit["admin:数据库并发验收"]
	if audit.Operator != "测试管理员" || audit.Reason != "独立测试 schema 验收" || p.Runes[audit.RuneUUID].UUID == "" {
		t.Fatal("审计与实际契印不一致")
	}
}

func TestPostgresIntimacyChapterRPCPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "好感度章节验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, "章节验收", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.Intimacy = map[int]int{4401: 32000}
		p.Materials[523] = game.Material{ID: 523, Count: 1, Total: 1}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := game.New(s, &hotfix.Catalog{})
	c := game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err := svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	args := []json.RawMessage{json.RawMessage(`51`), json.RawMessage(`4401`)}
	giftArgs := []json.RawMessage{json.RawMessage(`50`), json.RawMessage(`4401`), json.RawMessage(`523`), json.RawMessage(`1`), json.RawMessage(`0`)}
	if pushes, err := svc.Handle(ctx, c, "consume_intimacy_gift", giftArgs); err != nil || len(pushes) < 5 {
		t.Fatal("真实库送礼RPC失败", err, pushes)
	}
	pushes, err := svc.Handle(ctx, c, "receive_intimacy_bonus", args)
	if err != nil || len(pushes) < 5 || !reflect.DeepEqual(pushes[len(pushes)-1], game.Callback(51, []any{0})) {
		t.Fatal("真实库章节调用失败", err, pushes)
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := after.Avatars[0].Progress
	if len(p.Runes) != 4 || p.Materials[523].Count != 0 || p.Materials[523].Total != 1 || !reflect.DeepEqual(p.Runes, c.SelectedAvatarUnsafe().Progress.Runes) {
		t.Fatal("回礼契印或送礼扣费未持久化")
	}
	if p.IntimacyCommons[4401].RewardLevel != 1 || !reflect.DeepEqual(p.Materials, c.SelectedAvatarUnsafe().Progress.Materials) {
		t.Fatal("章节或奖励未持久化")
	}
	var dbraw []byte
	if err := s.pool.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, av.OID).Scan(&dbraw); err != nil {
		t.Fatal(err)
	}
	var persisted game.Progress
	if err := json.Unmarshal(dbraw, &persisted); err != nil || persisted.IntimacyCommons[4401].RewardLevel != 1 {
		t.Fatal("JSONB领取状态缺失", err)
	}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.IntimacyCommons[4401] = game.IntimacyCommon{RewardLevel: 5}
		p.OwnedHeadBox = map[int]float64{1023: 0}
		return errors.New("章节事务故障注入")
	}); err == nil {
		t.Fatal("故障注入未生效")
	}
	reloaded, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(p, reloaded.Avatars[0].Progress) {
		t.Fatal("章节故障没有整体回滚", err)
	}
}

func TestPostgresRuneLoginMigrationPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "契印迁移验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av := id.Avatars[0]
	runeID := "00112233445566778899aabb"
	badID := "00112233445566778899aabc"
	_, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.RuneSchemaVersion = 0
		p.Runes = map[string]game.Rune{runeID: {UUID: runeID, Suit: 1101, Star: 5, Position: 1, Level: 11}, badID: {UUID: badID, RuneID: 131101, Level: 1}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := first.Avatars[0].Progress
	if p.RuneSchemaVersion != game.CurrentRuneSchemaVersion || len(p.Runes) != 1 || len(p.Runes[runeID].BaseAttrs) != 2 || len(p.RuneQuarantine) != 1 {
		t.Fatal("数据库登录没有执行完整迁移")
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, av.OID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var persisted game.Progress
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Runes, persisted.Runes) || !reflect.DeepEqual(p.RuneQuarantine, persisted.RuneQuarantine) {
		t.Fatal("迁移仅存在内存，没有持久化")
	}
	second, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || !reflect.DeepEqual(p, second.Avatars[0].Progress) {
		t.Fatal("第二次登录重抽或改变存档", err)
	}
}

func TestPostgresProgressIdempotencyRollbackAndPersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "玩家进度测试", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av := id.Avatars[0]
	if av.UID <= 0 || av.Progress.Power.Value != 100 || av.Progress.Materials[12].Count != 10000 || av.Progress.GuideTasks[1000].Status != 1 {
		t.Fatalf("客户端初始化表未生效：%+v", av.Progress)
	}
	advance := func(p *game.Progress) error { return game.AdvanceGuide(p, 1000, time.Now(), av.Info.Level) }
	for i := 0; i < 2; i++ {
		if _, err = s.UpdateProgress(ctx, av.OID, advance); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Materials[12] = game.Material{ID: 12, Count: 999999}
		return errors.New("事务故障注入")
	}); err == nil {
		t.Fatal("回滚故障没有生效")
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	got := after.Avatars[0]
	if got.UID != av.UID || got.Progress.GuideTasks[1000].Status != 2 || got.Progress.GuideTasks[1001].Status != 1 || got.Progress.Materials[12].Count != 10000 {
		t.Fatal("进度未持久化或失败事务覆盖货币")
	}
	if _, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { return game.AdvanceGuide(p, 1008, time.Now(), 1) }); err == nil {
		t.Fatal("未激活任务可越级完成")
	}
	if _, err = s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error { p.GlobalVO = 1; p.StoryVO = 1; return nil }); err != nil {
		t.Fatal(err)
	}
	voice, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || voice.Avatars[0].Progress.GlobalVO != 1 || voice.Avatars[0].Progress.StoryVO != 1 {
		t.Fatal("语音设置没有持久化", err)
	}
}

func TestPostgresClientBattleLifecyclePersistence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info := game.ClientInfo{Account: "战斗启动验收", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av := id.Avatars[0]
	if av.NicknameSet || av.Progress.GuideTasks[1000].Status != 1 || len(av.Progress.Cards) != 1 || av.Progress.Cards[0].Grade != 0 {
		t.Fatal("教学PG用例必须从真实新账号创建态开始")
	}
	svc := game.New(s, &hotfix.Catalog{})
	now := time.Now()
	svc.Now = func() time.Time { return now }
	c := game.NewConnection()
	raw, _ := json.Marshal(info)
	if _, err = svc.Handle(ctx, c, "quick_login", []json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	defer svc.Detach(c)
	before, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "enter_dungeon", []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}); err == nil {
		t.Fatal("真实未取名新账号不应进入教学战斗")
	}
	afterRejected, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil || !reflect.DeepEqual(game.CloneProgress(before.Progress), game.CloneProgress(afterRejected.Progress)) {
		t.Fatal("未取名拒绝改变了新账号资产或引导", err)
	}
	// 原生客户端创建链：先取名RPC，成功后上报1000完成；不直接写资料或推进引导。
	out, err := svc.Handle(ctx, c, "set_nickname_gender", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`"战斗验收"`), json.RawMessage(`1`)})
	if err != nil {
		t.Fatal(err)
	}
	reply, at := pgNativeReply(t, out, "call_client_callback")
	if reply.Args[0] != 3 || reply.Args[1].([]any)[0] != game.RetSuccess {
		t.Fatal("原生取名失败", reply)
	}
	pgNativePropertyBefore(t, out, at, "nickname")
	pgNativePropertyBefore(t, out, at, "nickname_flag")
	var storedName string
	var storedFlag bool
	var storedGender int
	if err = s.pool.QueryRow(ctx, `SELECT nickname,nickname_set,gender FROM avatars WHERE avatar_oid=$1`, av.OID).Scan(&storedName, &storedFlag, &storedGender); err != nil || storedName != "战斗验收" || !storedFlag || storedGender != 1 {
		t.Fatal("原生取名未实际保存创建资料", err)
	}
	out, err = svc.Handle(ctx, c, "guide_task_finished", []json.RawMessage{json.RawMessage(`4`), json.RawMessage(`1000`)})
	if err != nil {
		t.Fatal(err)
	}
	reply, _ = pgNativeReply(t, out, "call_client_callback")
	if reply.Args[0] != 4 || reply.Args[1].([]any)[0] != true {
		t.Fatal("原生创建引导完成失败", reply)
	}
	created := c.SelectedAvatarUnsafe()
	if !created.NicknameSet || created.Progress.GuideTasks[1000].Status != 2 || created.Progress.GuideTasks[1001].Status != 1 {
		t.Fatal("新账号未按真实创建RPC进入教学阶段")
	}
	if _, err = svc.Handle(ctx, c, "enter_dungeon", []json.RawMessage{json.RawMessage(`9`), json.RawMessage(`10001`), json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "load_entity_finish", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Handle(ctx, c, "battle_fighting", []json.RawMessage{json.RawMessage(`{"fighting_cards":[0],"support_cards":[],"storyline_cards":[]}`)}); err != nil {
		t.Fatal(err)
	}
	p := c.SelectedAvatarUnsafe().Progress
	uuid := p.Battle.UUID
	for _, event := range []map[string]any{
		{"battle_uuid": uuid, "sequence": 1, "kind": "ready", "data": map[string]any{"version": 1}},
		{"battle_uuid": uuid, "sequence": 2, "kind": "started", "data": map[string]any{"units": []any{map[string]any{"eid": "1", "role": 4401, "hex": []int{0, 0, 0}, "kind": "ally", "hp": 100, "max_hp": 100}}}},
	} {
		raw, _ := json.Marshal([]any{event})
		if _, err := svc.Handle(ctx, c, "do_command", []json.RawMessage{json.RawMessage(`"__battle_event__"`), raw}); err != nil {
			t.Fatal(err)
		}
	}
	beforeTick := game.CloneProgress(c.SelectedAvatarUnsafe().Progress)
	now = now.Add(1500 * time.Millisecond)
	pushes, err := svc.Tick(ctx, c)
	if err != nil {
		t.Fatal("原生战斗Tick失败", err)
	}
	committed, err := New(s.pool).AdminPlayer(ctx, av.OID)
	if err != nil {
		t.Fatal(err)
	}
	expected := committed.InitialProperties(info.Account)
	// Tick会正常消费取名/社交事务提交后的刷新队列；只允许核对真实已提交属性，不能清队列避开断言。
	requiredRefresh := map[string]bool{"nickname": false, "nickname_flag": false, "gender": false, "level": false, "power": false, "material_mgr": false, "card_mgr": false}
	oldDrivers := map[string]bool{
		"revival_tick": true, "revival_do_command": true, "on_battle_start": true,
		"real_round_end": true, "real_start_next_round": true, "real_continue_old_round": true,
		"after_pre_play": true, "continue_wait_for_player_input": true, "battle_result": true,
	}
	for _, item := range pushes {
		if oldDrivers[item.Method] {
			t.Fatal("Tick下发旧权威驱动或未经结果事件授权的结算", item.Method)
		}
		if item.Method == "sync_battle_method" {
			if len(item.Args) == 0 {
				t.Fatal("Tick下发无方法名战斗调用")
			}
			method, ok := item.Args[0].(string)
			if !ok || oldDrivers[method] {
				t.Fatal("Tick下发旧权威调度/自动指令", item.Args[0])
			}
			// 已started且无新用户输入：Tick本身没有新的原生战斗调用授权。
			t.Fatal("Tick擅自下发原生战斗调用", method)
		}
		if item.Method != "client_prop_changed" {
			continue
		}
		if len(item.Args) != 1 {
			t.Fatal("提交后属性刷新参数错误", item)
		}
		property, ok := item.Args[0].([]any)
		if !ok || len(property) != 2 {
			t.Fatal("提交后属性刷新元组错误", item)
		}
		field, ok := property[0].(string)
		if !ok {
			t.Fatal("提交后属性刷新字段错误", property[0])
		}
		if _, required := requiredRefresh[field]; !required {
			continue
		}
		actualRaw, actualErr := json.Marshal(property[1])
		expectedRaw, expectedErr := json.Marshal(expected[field])
		if actualErr != nil || expectedErr != nil || !bytes.Equal(actualRaw, expectedRaw) {
			t.Fatal("Tick属性刷新不符真实PG已提交状态", field, actualErr, expectedErr)
		}
		requiredRefresh[field] = true
	}
	for field, seen := range requiredRefresh {
		if !seen {
			t.Fatal("取名提交后的合法属性刷新未送达", field)
		}
	}
	if !reflect.DeepEqual(beforeTick.Battle, game.CloneProgress(committed.Progress).Battle) {
		t.Fatal("Tick自动推进了原生战斗状态")
	}
	if committed.Progress.Power.Value != beforeTick.Power.Value || !reflect.DeepEqual(beforeTick.Materials, committed.Progress.Materials) || !reflect.DeepEqual(beforeTick.Cards, committed.Progress.Cards) {
		t.Fatal("等待原生输入的Tick擅自扣费或发奖")
	}
	after, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Battle.UUID != uuid || after.Avatars[0].Progress.Battle.LastSequence != 2 {
		t.Fatal("客户端生命周期未持久", err)
	}
	svc.Detach(c)
	svc, c = loginPersistedProgressPlayer(t, s, info, now)
	if _, err := svc.Handle(ctx, c, "client_need_recover_battle", nil); err != nil {
		t.Fatal(err)
	}
	after, err = New(s.pool).QuickLogin(ctx, info)
	if err != nil || after.Avatars[0].Progress.Battle.UUID == uuid || after.Avatars[0].Progress.Battle.Finished {
		t.Fatal("客户端战斗恢复未持久", err)
	}
}
