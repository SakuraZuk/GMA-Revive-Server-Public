// gate 传输适配层的端到端演练：按客户端指令流还原的握手与登录序列驱动状态机。
// 依据 out/gate-protocol-spec.md 与 out/login-flow-spec.md。
package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
)

func testService(t *testing.T) *game.Service {
	t.Helper()
	var catalog hotfix.Catalog
	if err := catalog.Load("../../deploy/data/hotfix.json"); err != nil {
		t.Fatal(err)
	}
	accounts := game.NewFixtureAccounts(map[string]game.FixtureAccount{
		"dev1": {Password: "pw1", Avatars: []game.Avatar{{Hostnum: 1, Info: game.AvatarInfo{Nickname: "n1", Level: 5}}}},
	})
	return game.New(accounts, &catalog)
}

func frame(t *testing.T, f mobileproto.Frame) []byte {
	t.Helper()
	wire, err := mobileproto.Encode(f, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

// TestGateHandshakeAndLogin 走完 seed → session_key → connect_server → quick_login 全链。
func TestGateHandshakeAndLogin(t *testing.T) {
	svc := testService(t)
	svc.Now = func() time.Time { return time.Unix(1700000000, 500000000) }
	g := newGate(1<<20, func(format string, v ...any) { t.Logf(format, v...) })
	conn := game.NewConnection()

	// 1. seed_request → seed_reply
	replies := g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdSeedRequest})
	if len(replies) != 1 || replies[0].Command != mobileproto.CmdSeedReply {
		t.Fatalf("seed_reply 缺失: %+v", replies)
	}
	fields, err := mobileproto.Fields(replies[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := mobileproto.First(fields, 1); !ok || f.Varint == 0 {
		t.Fatalf("seed_reply 字段 1 无效: %+v", fields)
	}

	// 2. session_key（明文联调模式：EncryptString{encryptstr=SessionKey 序列化}）
	var sk []byte
	sk = mobileproto.AppendField(sk, 2, 2, []byte("0123456789abcdefghij")) // session_key=20B
	encStr := mobileproto.AppendField(nil, 1, 2, sk)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdSessionKey, Payload: encStr})
	if len(replies) != 1 || replies[0].Command != mobileproto.CmdSessionKeyOK {
		t.Fatalf("session_key_ok 缺失: %+v", replies)
	}
	g.mu.Lock()
	hasCrypt := g.read != nil && g.write != nil
	g.mu.Unlock()
	if !hasCrypt {
		t.Fatal("session_key 后 RC4 双向未启用")
	}

	// 3. connect_server(NEW_CONNECTION) → connect_reply(CONNECTED) + create_entity(Account)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdConnectServer,
		Payload: mobileproto.AppendVarintField(nil, 2, 0)})
	if len(replies) != 2 || replies[0].Command != mobileproto.CmdConnectReply || replies[1].Command != mobileproto.CmdCreateEntity {
		t.Fatalf("connect_server 回包序列错误: %+v", replies)
	}
	crFields, _ := mobileproto.Fields(replies[0].Payload)
	if f, ok := mobileproto.First(crFields, 2); !ok || f.Varint != 1 {
		t.Fatalf("connect_reply.type != CONNECTED: %+v", crFields)
	}

	// 4. quick_login(client_info) → login_result(0) + on_get_all_avatars + on_hotfix_when_login
	//    + create_entity(Avatar) + sync_server_time（on_refresh_login 由收尾上行触发，见第 9 步）
	clientInfo := map[string]any{
		"hostnum": int64(1), "account": "dev1", "password": "pw1",
		"hotfix_index": int64(0), "need_guide_ids": []any{}, "conn_type": int64(0),
	}
	// 客户端上行参数为 {"_0": client_info}（实测 bin 键 "_0"）。
	params := mobileproto.EncodeMsgpackMap(map[string]any{"_0": clientInfo})
	em := mobileproto.EncodeEntityMessage(g.accountID, "quick_login", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	var methods []string
	for _, r := range replies {
		if r.Command != mobileproto.CmdPushEntity && r.Command != mobileproto.CmdCreateEntity {
			t.Fatalf("意外命令 %d", r.Command)
		}
		if r.Command == mobileproto.CmdCreateEntity {
			fields, _ := mobileproto.Fields(r.Payload)
			id, _ := mobileproto.First(fields, 3)
			selected, ok := conn.SelectedAvatar()
			if !ok || !bytes.Equal(id.Bytes, selected.OID) {
				t.Fatal("实体没有绑定鉴权角色编号")
			}
			info, _ := mobileproto.First(fields, 4)
			body, err := mobileproto.DecodeMsgpackToJSON(info.Bytes)
			var properties map[string]json.RawMessage
			if err != nil || json.Unmarshal(body, &properties) != nil {
				t.Fatal("玩家属性无法解码")
			}
			var power game.Power
			if json.Unmarshal(properties["power"], &power) != nil || power.Value != 100 || power.Interval != 300 || power.LastTime <= 0 {
				t.Fatalf("体力契约无效：%s", body)
			}
			var tasks map[int]game.GuideTask
			if json.Unmarshal(properties["guide_tasks"], &tasks) != nil || tasks[1000].Status != 1 {
				t.Fatal("新角色没有执行态引导")
			}
			methods = append(methods, "create_entity:Avatar")
			continue
		}
		msg, err := mobileproto.DecodeEntityMessage(r.Payload)
		if err != nil {
			t.Fatal(err)
		}
		methods = append(methods, msg.MethodName())
		if msg.MethodName() == "sync_server_time" {
			raw, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
			var doc map[string]float64
			if err != nil || json.Unmarshal(raw, &doc) != nil || doc["_0"] != 1700000000.5 || doc["_1"] != -28800 {
				t.Fatalf("时间编码不符：%s %v", raw, err)
			}
		}
	}
	want := []string{
		"login_result", "on_get_all_avatars", "on_hotfix_when_login",
		"create_entity:Avatar", "sync_server_time",
	}
	if len(methods) != len(want) {
		t.Fatalf("登录推送序列不符:\n得到 %v\n期望 %v", methods, want)
	}
	for i := range want {
		if methods[i] != want[i] {
			t.Fatalf("登录推送第 %d 项 %s != %s", i, methods[i], want[i])
		}
	}

	// 5. 登录后 query_hotfix(0) → on_query_hotfix_success
	params = mobileproto.EncodeMsgpackMap(map[string]any{"_0": int64(0)})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "query_hotfix", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 1 {
		t.Fatalf("query_hotfix 回包数 %d", len(replies))
	}
	msg, _ := mobileproto.DecodeEntityMessage(replies[0].Payload)
	if msg.MethodName() != "on_query_hotfix_success" {
		t.Fatalf("query_hotfix 回包 %s", msg.MethodName())
	}

	// 7. 登录后的真实首批业务调用：测速启动必须回传 Int，消息板空列表必须可解码。
	for _, method := range []string{"start_speed_check", "query_league_message_board"} {
		em = mobileproto.EncodeEntityMessage(g.avatarID, method, mobileproto.EncodeMsgpackMap(map[string]any{}), true)
		replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
		if len(replies) != 1 {
			t.Fatalf("%s 回包数 %d", method, len(replies))
		}
		msg, err = mobileproto.DecodeEntityMessage(replies[0].Payload)
		if err != nil {
			t.Fatalf("%s 回包解码失败：%v", method, err)
		}
		wantMethod := method
		if method == "query_league_message_board" {
			wantMethod = "on_query_league_message_board"
		}
		if msg.MethodName() != wantMethod {
			t.Fatalf("%s 回包方法 %s", method, msg.MethodName())
		}
		body, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
		if err != nil {
			t.Fatalf("%s 参数解码失败：%v", method, err)
		}
		var args map[string]json.RawMessage
		if err := json.Unmarshal(body, &args); err != nil {
			t.Fatalf("%s 参数不是字典：%s", method, body)
		}
		if method == "start_speed_check" {
			var checkType int
			if err := json.Unmarshal(args["_0"], &checkType); err != nil || checkType != 1 {
				t.Fatalf("测速 check_type 错误：%s", body)
			}
		} else if len(args) != 1 || string(args["_0"]) != "[]" {
			t.Fatalf("空消息板编码错误：%s", body)
		}
	}

	// 8. set_send_rpc_salt 静默忽略
	em = mobileproto.EncodeEntityMessage(g.avatarID, "set_send_rpc_salt", params, true)
	if out := g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em}); len(out) != 0 {
		t.Fatalf("set_send_rpc_salt 应被忽略，实际回包 %d 条", len(out))
	}
	// 无参 RPC 的客户端空参数字典必须正常展开为空列表。
	params = mobileproto.EncodeMsgpackMap(map[string]any{})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "query_server_time", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 1 {
		t.Fatalf("空字典时间查询未响应：%v", replies)
	}

	// 9. 客户端收尾上行 set_reconnect_auth_msg → on_refresh_login + on_guide_task_condition_happened(1,1)
	//    （主城初始化链入口与新手引导链起点）；重复上行不再推送（防循环）。
	params = mobileproto.EncodeMsgpackMap(map[string]any{"_0": "重连凭证"})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "set_reconnect_auth_msg", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 1 {
		t.Fatalf("重连凭证回包数 %d", len(replies))
	}
	msg, _ = mobileproto.DecodeEntityMessage(replies[0].Payload)
	if msg.MethodName() != "on_refresh_login" {
		t.Fatalf("重连凭证应回 on_refresh_login，实际 %s", msg.MethodName())
	}
	refreshJSON, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
	if err != nil {
		t.Fatalf("刷新登录参数解码失败：%v", err)
	}
	var refreshArgs map[string]json.RawMessage
	if err := json.Unmarshal(refreshJSON, &refreshArgs); err != nil || len(refreshArgs) != 0 {
		t.Fatalf("刷新登录应为空参数：%s", refreshJSON)
	}
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 0 {
		t.Fatalf("重复重连凭证不应再推刷新登录：%d", len(replies))
	}

	// 10. 心跳上行 heart_beat(last_send_time) → 下行 heart_beat(server_time, last_send_time)；
	//     两个时间参数都必须保留 Float 精度（客户端做减法算延迟）。
	params = mobileproto.EncodeMsgpackMap(map[string]any{"_0": 1700000001.25})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "heart_beat", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 1 {
		t.Fatalf("心跳回包数 %d", len(replies))
	}
	msg, _ = mobileproto.DecodeEntityMessage(replies[0].Payload)
	if msg.MethodName() != "heart_beat" {
		t.Fatalf("心跳回包方法 %s", msg.MethodName())
	}
	hbJSON, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
	if err != nil {
		t.Fatalf("心跳参数解码失败：%v", err)
	}
	var hbArgs map[string]json.RawMessage
	if err := json.Unmarshal(hbJSON, &hbArgs); err != nil {
		t.Fatalf("心跳参数不是字典：%s", hbJSON)
	}
	if string(hbArgs["_0"]) != "1700000000.5" || string(hbArgs["_1"]) != "1700000001.25" {
		t.Fatalf("心跳时间参数错误：%s", hbJSON)
	}

	// 11. 引导完成上行 guide_task_finished(callback_id, task_id) → call_client_callback(callback_id,[true,""])；
	//     upload_guide_tasks(callback_id, tasks) 只记录不回包（客户端 callback 为 None）。
	params = mobileproto.EncodeMsgpackMap(map[string]any{"_0": int64(7), "_1": int64(1000)})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "guide_task_finished", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 1 {
		t.Fatalf("引导完成回包数 %d", len(replies))
	}
	msg, _ = mobileproto.DecodeEntityMessage(replies[0].Payload)
	if msg.MethodName() != "call_client_callback" {
		t.Fatalf("引导完成应回 call_client_callback，实际 %s", msg.MethodName())
	}
	cbJSON, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
	if err != nil {
		t.Fatalf("引导回调参数解码失败：%v", err)
	}
	var cbArgs map[string]json.RawMessage
	if err := json.Unmarshal(cbJSON, &cbArgs); err != nil || string(cbArgs["_0"]) != "7" {
		t.Fatalf("引导回调编号错误：%s", cbJSON)
	}
	params = mobileproto.EncodeMsgpackMap(map[string]any{"_0": int64(8), "_1": map[string]any{}})
	em = mobileproto.EncodeEntityMessage(g.avatarID, "upload_guide_tasks", params, true)
	replies = g.handleFrame(svc, conn, nil, mobileproto.Frame{Command: mobileproto.CmdEntityMessage, Payload: em})
	if len(replies) != 0 {
		t.Fatalf("引导状态上报不应回包：%d", len(replies))
	}
}

// TestGateRC4RoundTrip 验证手写 RC4 与标准测试向量一致（RFC 6229 风格冒烟）。
func TestGateRC4RoundTrip(t *testing.T) {
	key := []byte("Key")
	data := []byte("Plaintext")
	want := []byte{0xbb, 0xf3, 0x16, 0xe8, 0xd9, 0x40, 0xaf, 0x0a, 0xd3}
	c := newRC4(key)
	got := make([]byte, len(data))
	c.xor(got, data)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RC4 向量不符: got %x want %x", got, want)
		}
	}
	// 同 key 解密还原
	d := newRC4(key)
	back := make([]byte, len(got))
	d.xor(back, got)
	if string(back) != string(data) {
		t.Fatal("RC4 解密不一致")
	}
}
