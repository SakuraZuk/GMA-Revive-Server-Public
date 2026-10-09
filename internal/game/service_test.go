package game

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
)

func TestGuideFailureTipUsesNativeUTF8BytesWithoutAdvancing(t *testing.T) {
	ctx := context.Background()
	accounts := NewFixtureAccounts(nil)
	svc := New(accounts, nil)
	c, _ := newBattleConnection(t, ctx, accounts, svc)
	before, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	out, err := svc.Handle(ctx, c, "guide_task_finished", rawArgs(73, 999999))
	if err != nil || len(out) != 1 || out[0].Method != "call_client_callback" {
		t.Fatal("拒绝回调丢失", err, out)
	}
	args := out[0].Args[1].([]any)
	message, ok := args[1].([]byte)
	if args[0] != false || !ok || !utf8.Valid(message) || string(message) != "引导任务不存在或未激活" {
		t.Fatal("原生错误提示类型错误", args)
	}
	encoded := mobileproto.EncodeMsgpackValue(nil, message)
	if encoded[0] != 0xc4 {
		t.Fatal("UTF-8字节没有编码为bin", encoded)
	}
	after, _ := json.Marshal(c.SelectedAvatarUnsafe().Progress)
	if !bytes.Equal(before, after) {
		t.Fatal("拒绝改变教学或资产")
	}
}

func fixture(t *testing.T) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hotfix.json")
	if err := os.WriteFile(path, []byte(`{"startup_scripts":{"1.0.125":"pass\n"},"runtime":{"index":2,"script":"pass\n"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var c hotfix.Catalog
	if err := c.Load(path); err != nil {
		t.Fatal(err)
	}
	s := New(NewFixtureAccounts(map[string]FixtureAccount{"local": {Password: "pw", Avatars: []Avatar{{Hostnum: 10001, Info: AvatarInfo{Nickname: "测试角色", Level: 1}}}}}), &c)
	s.Now = func() time.Time { return time.Unix(1700000000, 500000000) }
	return s
}

func TestConcurrentFixtureRegistrationAndUnicodeAccount(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	// 注册是唯一建号入口：显式动作建号后，登录只认既有账号。
	if _, err := accounts.QuickLogin(context.Background(), ClientInfo{Account: "甲", Password: "pw", Hostnum: 1}); err == nil {
		t.Fatal("未知账号登录未被拒绝")
	}
	if _, err := accounts.Register(context.Background(), ClientInfo{Account: "甲", Password: "pw", Hostnum: 1}); err != nil {
		t.Fatalf("显式注册失败：%v", err)
	}
	if _, err := accounts.Register(context.Background(), ClientInfo{Account: "甲", Password: "pw2", Hostnum: 1}); err == nil {
		t.Fatal("重复注册未被拒绝")
	}
	var wg sync.WaitGroup
	ids := make(chan []byte, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity, err := accounts.QuickLogin(context.Background(), ClientInfo{Account: "甲", Password: "pw", Hostnum: 1})
			if err != nil {
				t.Error(err)
				return
			}
			if identity.Avatars[0].Info.Nickname != "书灵甲" {
				t.Error("中文短账号昵称错误")
			}
			ids <- identity.Avatars[0].OID
		}()
	}
	wg.Wait()
	close(ids)
	var oid []byte
	for id := range ids {
		if oid == nil {
			oid = id
		}
		if !bytes.Equal(oid, id) {
			t.Fatal("同账号并发创建了不同角色")
		}
	}
	if _, err := accounts.QuickLogin(context.Background(), ClientInfo{Account: "甲", Password: "错误", Hostnum: 1}); err == nil {
		t.Fatal("错误密码未拒绝")
	}
	seen := make(map[string]bool)
	now := time.Unix(1700000000, 0)
	for i := 0; i < 1000; i++ {
		id := NewAvatarOID(now)
		if len(id) != 12 || seen[string(id)] {
			t.Fatal("同时刻角色标识碰撞")
		}
		seen[string(id)] = true
	}
}

func TestRegisterLoginGateAndAccountErrors(t *testing.T) {
	s := fixture(t)
	// register_login 成功序列与 quick_login 相同：成功结果 + 角色列表 + 热修。
	c := NewConnection()
	pushes, err := s.Handle(context.Background(), c, "register_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "新号", Password: "pw", ConnType: 3}))
	if err != nil || len(pushes) != 3 || pushes[0].Method != "login_result" || pushes[0].Args[0] != RetSuccess || c.Phase() != Authenticated {
		t.Fatalf("注册登录序列错误：%v %v", pushes, err)
	}
	// 重复注册回 9012，连接关闭且不下发角色。
	// （quick_login 未知账号已恢复设备账号自动注册，见 TestQuickLoginRegisterInfoCreatesAccount。）
	{
		c := NewConnection()
		args := rawArgs(ClientInfo{Hostnum: 10001, Account: "新号", Password: "任意"})
		pushes, err := s.Handle(context.Background(), c, "register_login", args)
		if err != nil || len(pushes) != 1 || pushes[0].Args[0] != RetAccountError || c.Phase() != Closed {
			t.Fatalf("register_login 失败序列错误：%v %v", pushes, err)
		}
		if _, err := s.BecomePlayer(c); err == nil {
			t.Fatal("失败鉴权后仍可成为玩家")
		}
	}
}

// 客户端真实注册路径（B9F5C696）：无独立 register_login 上行。
// ①quick_login + register_info 附加字段 = 真实输入 UI 的显式注册；
// ②无标记的未知设备账号自动注册 = 原厂"设备快速账号"语义（重装设备新 hs_<uuid> 进服路径）。
func TestQuickLoginRegisterInfoCreatesAccount(t *testing.T) {
	s := fixture(t)
	info := ClientInfo{Hostnum: 10001, Account: "输入框新号", Password: "pw", ConnType: 3, RegisterInfo: json.RawMessage(`{}`)}
	c := NewConnection()
	pushes, err := s.Handle(context.Background(), c, "quick_login", rawArgs(info))
	if err != nil || len(pushes) != 3 || pushes[0].Args[0] != RetSuccess || c.Phase() != Authenticated {
		t.Fatalf("quick_login+register_info 注册序列错误：%v %v", pushes, err)
	}
	// 同账号再走一次 quick_login（无 register_info）应作为既有账号登录成功。
	again := NewConnection()
	pushes, err = s.Handle(context.Background(), again, "quick_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "输入框新号", Password: "pw"}))
	if err != nil || len(pushes) != 3 || pushes[0].Args[0] != RetSuccess {
		t.Fatalf("注册后的账号应可登录：%v %v", pushes, err)
	}
	// 无 register_info 的未知设备账号自动注册（hotfix-e2e 2026-10-06：重装设备 9012 卡死修复）。
	stranger := NewConnection()
	pushes, err = s.Handle(context.Background(), stranger, "quick_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "重装设备新号", Password: "pw"}))
	if err != nil || len(pushes) != 3 || pushes[0].Args[0] != RetSuccess || stranger.Phase() != Authenticated {
		t.Fatalf("未知设备账号应自动注册：%v %v", pushes, err)
	}
}

func TestGuardSpeedCheckAndLeagueMessageBoard(t *testing.T) {
	s, c := fixture(t), NewConnection()
	if _, err := s.Handle(context.Background(), c, "start_speed_check", nil); err == nil {
		t.Fatal("未成为玩家可启动测速")
	}
	if _, err := s.Handle(context.Background(), c, "query_league_message_board", nil); err == nil {
		t.Fatal("未成为玩家可查询消息板")
	}
	if _, err := s.Handle(context.Background(), c, "quick_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "local", Password: "pw", ConnType: 3})); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BecomePlayer(c); err != nil {
		t.Fatal(err)
	}
	pushes, err := s.Handle(context.Background(), c, "start_speed_check", nil)
	if err != nil || len(pushes) != 1 || pushes[0].Target != "Avatar" || pushes[0].Method != "start_speed_check" || pushes[0].Args[0] != 1 {
		t.Fatalf("测速启动应答错误：%v %v", pushes, err)
	}
	pushes, err = s.Handle(context.Background(), c, "start_speed_check", nil)
	if err != nil || pushes[0].Args[0] != 2 {
		t.Fatalf("测速轮次未递增：%v %v", pushes, err)
	}
	if pushes, err = s.Handle(context.Background(), c, "speed_check", rawArgs(2, 0, 60)); err != nil || len(pushes) != 0 {
		t.Fatalf("测速上报不应回包：%v %v", pushes, err)
	}
	if _, err := s.Handle(context.Background(), c, "speed_check", rawArgs("坏", 0, 60)); err == nil {
		t.Fatal("非数字测速参数未被拒绝")
	}
	if _, err := s.Handle(context.Background(), c, "speed_check", rawArgs(2, 0)); err == nil {
		t.Fatal("缺参测速上报未被拒绝")
	}
	pushes, err = s.Handle(context.Background(), c, "query_league_message_board", nil)
	if err != nil || len(pushes) != 1 || pushes[0].Target != "Avatar" || pushes[0].Method != "on_query_league_message_board" {
		t.Fatalf("消息板应答方法错误：%v %v", pushes, err)
	}
	board, ok := pushes[0].Args[0].([]any)
	if !ok || len(board) != 0 {
		t.Fatalf("未入公会应回空消息板：%v", pushes[0].Args[0])
	}
}

func rawArgs(args ...any) []json.RawMessage {
	result := make([]json.RawMessage, len(args))
	for i, a := range args {
		result[i], _ = json.Marshal(a)
	}
	return result
}

func TestLoginSequenceAndRuntimeHotfix(t *testing.T) {
	s, c := fixture(t), NewConnection()
	if _, err := s.Handle(context.Background(), c, "query_hotfix", rawArgs(0)); err == nil {
		t.Fatal("未登录可查询热修")
	}
	pushes, err := s.Handle(context.Background(), c, "quick_login", rawArgs(ClientInfo{Hostnum: 10001, Account: "local", Password: "pw", ConnType: 3}))
	if err != nil || len(pushes) != 3 || pushes[0].Method != "login_result" || pushes[0].Args[0] != 0 || len(pushes[0].Args) != 3 || pushes[1].Method != "on_get_all_avatars" || pushes[2].Args[1] != 2 {
		t.Fatalf("登录序列错误：%v %v", pushes, err)
	}
	pushes, err = s.BecomePlayer(c)
	if err != nil || len(pushes) != 1 || pushes[0].Method != "sync_server_time" || pushes[0].Args[0] != float64(1700000000.5) || pushes[0].Args[1] != -28800 {
		t.Fatal("玩家及时间推送序列错误")
	}
	// 客户端收尾上行 set_reconnect_auth_msg 触发 on_refresh_login 与引导链起点事件；
	// 重复上行不再推送（防循环）。
	pushes, err = s.Handle(context.Background(), c, "set_reconnect_auth_msg", rawArgs("重连凭证"))
	if err != nil || len(pushes) != 1 || pushes[0].Target != "Avatar" || pushes[0].Method != "on_refresh_login" || len(pushes[0].Args) != 0 {
		t.Fatalf("重连凭证后推送序列错误：%v %v", pushes, err)
	}
	pushes, err = s.Handle(context.Background(), c, "set_reconnect_auth_msg", rawArgs("重连凭证2"))
	if err != nil || len(pushes) != 0 {
		t.Fatalf("刷新登录不应重复推送：%v %v", pushes, err)
	}
	// 引导完成上报必须回 call_client_callback(callback_id, [success, msg])，否则客户端引导链卡住。
	pushes, err = s.Handle(context.Background(), c, "guide_task_finished", rawArgs(7, 1000))
	if err != nil || len(pushes) != 1 || pushes[0].Method != "call_client_callback" || pushes[0].Args[0] != 7 {
		t.Fatalf("引导完成应答错误：%v %v", pushes, err)
	}
	// 引导状态上报只记录，不回包（客户端 callback 为 None）。
	pushes, err = s.Handle(context.Background(), c, "upload_guide_tasks", rawArgs(8, map[string]any{}))
	if err != nil || len(pushes) != 0 {
		t.Fatalf("引导状态上报不应回包：%v %v", pushes, err)
	}
	// 心跳：上行 last_send_time，下行必须回 server_time 与 last_send_time（客户端延迟计算）。
	pushes, err = s.Handle(context.Background(), c, "heart_beat", rawArgs(1700000001.25))
	if err != nil || len(pushes) != 1 || pushes[0].Target != "Avatar" || pushes[0].Method != "heart_beat" ||
		pushes[0].Args[0] != float64(1700000000.5) || pushes[0].Args[1] != 1700000001.25 {
		t.Fatalf("心跳应答应回 server_time 与 last_send_time：%v %v", pushes, err)
	}
	for _, index := range []int{0, 2, 3} {
		pushes, err = s.Handle(context.Background(), c, "query_hotfix", rawArgs(index))
		if err != nil || pushes[0].Method != "on_query_hotfix_success" {
			t.Fatal("热修响应错误")
		}
		if index >= 2 && (pushes[0].Args[0] != "" || pushes[0].Args[1] != index) {
			t.Fatal("无更新不应降级客户端")
		}
	}
	if _, err := s.BecomePlayer(c); err == nil {
		t.Fatal("重复玩家绑定未拒绝")
	}
	if got := Callback(42, []any{"结果"}); got.Method != "call_client_callback" || got.Args[0] != 42 {
		t.Fatal("回调编号丢失")
	}
	if got := Kick(c, 4, "封禁原因"); got.Args[1] != "封禁原因" || c.Phase() != Closed {
		t.Fatal("踢人未关闭业务连接")
	}
}

func TestFailedAuthDoesNotPushRolesOrPlayer(t *testing.T) {
	s := fixture(t)
	for _, method := range []string{"quick_login", "sdk_login"} {
		c := NewConnection()
		args := rawArgs(ClientInfo{Hostnum: 10001, Account: "local", Password: "wrong"})
		if method == "sdk_login" {
			args = append(args, json.RawMessage(`{}`))
		}
		pushes, err := s.Handle(context.Background(), c, method, args)
		if err != nil || len(pushes) != 1 || pushes[0].Args[0] != RetAuthFailed || c.Phase() != Closed {
			t.Fatal("失败鉴权序列错误")
		}
		if _, err := s.BecomePlayer(c); err == nil {
			t.Fatal("失败鉴权后仍可成为玩家")
		}
	}
}
