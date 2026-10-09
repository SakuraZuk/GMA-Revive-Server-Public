// gameserver 的移动端传输适配层：MobileRPC 帧 ↔ internal/game 业务层。
// 协议依据 out/gate-protocol-spec.md（2026-10-05 mbengine 反汇编定稿）：
//
//	seed_request → seed_reply → session_key(RSA-OAEP 包 SHA1 key) → session_key_ok
//	→ connect_server → connect_reply + create_entity(Account) → entity_message 业务
package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/mobileproto"
)

// gate 是单条 TCP 连接的传输状态；RC4 在 session_key 后启用。
type gate struct {
	mu          sync.Mutex
	read, write *rc4
	avatarReady bool
	accountID   []byte
	avatarID    []byte
	avatarInfo  game.AvatarInfo
	avatarProps map[string]any
	maxPayload  int
	logf        func(string, ...any)
}

func newGate(maxPayload int, logf func(string, ...any)) *gate {
	return &gate{
		maxPayload: maxPayload,
		logf:       logf,
		accountID:  entityID(1),
		avatarID:   entityID(2),
	}
}

// entityID 生成 12 字节 bson ObjectId 兼容 id（客户端 IdManager.bytes2id=ObjectId(bytes) 要求 12B）。
func entityID(v uint64) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[:4], uint32(time.Now().Unix()))
	binary.BigEndian.PutUint64(b[4:], v)
	return b
}

// loadPrivateKey 读取 PKCS#1/PKCS#8 PEM 私钥；路径为空返回 nil（未配置时按明文 SessionKey 解析）。
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, fmt.Errorf("私钥 %s 不是 PEM 格式", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 RSA 私钥 %s: %w", path, err)
	}
	key, ok := anyKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("私钥不是 RSA")
	}
	return key, nil
}

// handleFrame 处理一条已解密帧，返回需写回的明文帧。
func (g *gate) handleFrame(svc *game.Service, conn *game.Connection, key *rsa.PrivateKey, f mobileproto.Frame) []mobileproto.Frame {
	switch f.Command {
	case mobileproto.CmdSeedRequest:
		var seed [8]byte
		_, _ = rand.Read(seed[:])
		seedVal := binary.LittleEndian.Uint64(seed[:])
		g.logf("发送 seed_reply seed=%d", seedVal)
		return []mobileproto.Frame{{Command: mobileproto.CmdSeedReply,
			Payload: mobileproto.AppendVarintField(nil, 1, seedVal)}}
	case mobileproto.CmdSessionKey:
		enc, err := mobileproto.DecodeEncryptString(f.Payload)
		if err != nil {
			return nil
		}
		clear := enc
		if key != nil {
			// 客户端 LoginKeyEncrypterNokeyczar：RSA PKCS1_OAEP（SHA1 默认空标签）。
			if clear, err = rsa.DecryptOAEP(sha1.New(), rand.Reader, key, enc, nil); err != nil {
				g.logf("session_key RSA 解密失败: %v", err)
				return nil
			}
		}
		sessionKey, seedBack, err := mobileproto.DecodeSessionKey(clear)
		g.logf("session_key 解密成功 key=%dB 客户端回传 seed=%d", len(sessionKey), seedBack)
		if err != nil || len(sessionKey) == 0 {
			g.logf("session_key 结构无效（len=%d err=%v）", len(clear), err)
			return nil
		}
		g.mu.Lock()
		g.read = newRC4(sessionKey)
		if g.write == nil {
			g.write = newRC4(sessionKey)
		}
		g.mu.Unlock()
		return []mobileproto.Frame{{Command: mobileproto.CmdSessionKeyOK}}
	case mobileproto.CmdConnectServer:
		req, err := mobileproto.DecodeConnectRequest(f.Payload)
		if err != nil {
			g.logf("connect_server 解码失败: %v", err)
			return nil
		}
		g.logf("connect_server type=%d device=%q", req.Type, req.DeviceID)
		if req.Type == 1 {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			avatar, replay, err := svc.Resume(ctx, conn, req.EntityID, string(req.DeviceID), req.AuthMsg)
			if err != nil {
				g.logf("重连拒绝: %v", err)
				return []mobileproto.Frame{{Command: mobileproto.CmdConnectReply, Payload: mobileproto.EncodeConnectReply(3, nil)}}
			}
			g.avatarID, g.avatarInfo, g.avatarReady = avatar.OID, avatar.Info, true
			g.logf("重连恢复原 Avatar 成功 id=%x", avatar.OID)
			// _deal_reconnect_reply 会重设原 Avatar 的 server_proxy；不要重建实体或播放离场视频。
			// connect_reply(2) 与战斗阶段重放跳转必须整批写出（RC4 流顺序）。
			frames := []mobileproto.Frame{{Command: mobileproto.CmdConnectReply, Payload: mobileproto.EncodeConnectReply(2, avatar.OID)}}
			return append(frames, g.encodePushes(replay)...)
		}
		if req.Type != 0 || conn.Phase() != game.Connected {
			return []mobileproto.Frame{{Command: mobileproto.CmdConnectReply, Payload: mobileproto.EncodeConnectReply(4, nil)}}
		}
		conn.SetDeviceID(string(req.DeviceID))
		payload := mobileproto.EncodeConnectReply(1 /*CONNECTED*/, g.accountID)
		return []mobileproto.Frame{
			{Command: mobileproto.CmdConnectReply, Payload: payload},
			{Command: mobileproto.CmdCreateEntity,
				Payload: mobileproto.EncodeCreateEntity("Account", g.accountID, mobileproto.EncodeMsgpackMap(map[string]any{}))},
		}
		// 联调诊断：逐步启用下行帧以定位客户端分发差异。
	case mobileproto.CmdEntityMessage:
		return g.handleEntityMessage(svc, conn, f.Payload)
	}
	return nil
}

func (g *gate) handleEntityMessage(svc *game.Service, conn *game.Connection, payload []byte) []mobileproto.Frame {
	msg, err := mobileproto.DecodeEntityMessage(payload)
	if err != nil {
		return nil
	}
	method := msg.MethodName()
	// 实体归属来自该连接鉴权结果；不能用另一角色的 ID 发业务。
	expectedID := g.accountID
	if conn.Phase() == game.Playing {
		expectedID = g.avatarID
	}
	if !bytes.Equal(msg.ID, expectedID) {
		g.logf("entity_message 拒绝不属于当前连接的实体 id=%x", msg.ID)
		return nil
	}
	if method == "" {
		g.logf("entity_message: 未知方法 index=%d", msg.MethodIdx)
		return nil
	}
	if method == "set_send_rpc_salt" {
		// 客户端 create_entity 后必回的 salt 交接；服务端不使用 index，直接忽略。
		return nil
	}
	var args []json.RawMessage
	if len(msg.Parameters) > 0 {
		raw, err := mobileproto.DecodeMsgpackToJSON(msg.Parameters)
		if err != nil {
			g.logf("entity_message %s 参数 msgpack 解码失败: %v", method, err)
			return nil
		}
		if args, err = normalizeArgs(raw); err != nil {
			g.logf("entity_message %s 参数归一失败: %v", method, err)
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	pushes, err := svc.Handle(ctx, conn, method, args)
	if err != nil {
		avatar, _ := conn.SelectedAvatar()
		g.logf("entity_message %s 业务失败 uid=%d oid=%x: %v", method, avatar.UID, g.avatarID, err)
		// 只记录不含鉴权信息的故障定位参数，避免完整登录参数进入日志。
		if method == "enter_dungeon" || method == "random_cards" || method == "guide_task_finished" || method == "finished_guide" || method == "query_rank_list" {
			raw, _ := json.Marshal(args)
			if len(raw) > 512 {
				raw = raw[:512]
			}
			g.logf("业务定位 method=%s 参数=%s", method, raw)
		}
		return nil
	}
	// 单角色自动进游戏：账号鉴权完成即创建 Avatar 并成为玩家（login-flow-spec 登录收尾序列）。
	if method == "quick_login" || method == "sdk_login" || method == "register_login" {
		if conn.Phase() == game.Authenticated {
			avatar, ok := conn.SelectedAvatar()
			if !ok || len(avatar.OID) != 12 {
				g.logf("所选服务器缺少有效的十二字节角色标识")
				return nil
			}
			g.avatarID, g.avatarInfo = avatar.OID, avatar.Info
			g.logf("玩家绑定 uid=%d oid=%x hostnum=%d", avatar.UID, avatar.OID, avatar.Hostnum)
			g.avatarProps = jsonInts(svc.AvatarProperties(avatar, avatar.Account)).(map[string]any)
			// time_auto_attr.last_time 为 Float，保留秒精度；其余编号和计数用 Int。
			g.avatarProps["power"].(map[string]any)["last_time"] = avatar.Progress.Power.LastTime
			enter, err := svc.BecomePlayer(conn)
			if err != nil {
				g.logf("成为玩家失败: %v", err)
			} else {
				pushes = append(pushes, enter...)
			}
		}
	}
	return g.encodePushes(pushes)
}

// encodePushes 把业务推送编为 S→C entity_message 帧；
// 首个面向 Avatar 的推送前补 create_entity(Avatar)，构成单角色自动进游戏序列
// （quick_login 成功 → Avatar 实体 → on_login_success/on_become_player/sync_server_time）。
func (g *gate) encodePushes(pushes []game.Push) []mobileproto.Frame {
	var out []mobileproto.Frame
	for _, p := range pushes {
		if p.Method == "on_login_success" || p.Method == "on_become_player" {
			continue
		}
		id := g.accountID
		if p.Target == "Avatar" {
			g.mu.Lock()
			ready := g.avatarReady
			g.mu.Unlock()
			if !ready {
				properties := g.avatarProps
				if properties == nil {
					properties = map[string]any{"uid": int64(1), "nickname": g.avatarInfo.Nickname, "level": g.avatarInfo.Level, "head_id": g.avatarInfo.HeadID, "head_box_id": g.avatarInfo.HeadBoxID, "custom_head_image_url": g.avatarInfo.CustomHeadImageURL}
				}
				converted, err := wireValue(properties, false)
				if err != nil {
					g.logf("玩家初始属性编码失败：%v", err)
					continue
				}
				properties = converted.(map[string]any)
				out = append(out, mobileproto.Frame{Command: mobileproto.CmdCreateEntity,
					Payload: mobileproto.EncodeCreateEntity("Avatar", g.avatarID, mobileproto.EncodeMsgpackMap(properties))})
				g.mu.Lock()
				g.avatarReady = true
				g.mu.Unlock()
			}
			id = g.avatarID
		}
		args := make([]any, len(p.Args))
		valid := true
		for i, value := range p.Args {
			// Float 签名的时间保留浮点类型与精度，其他 UI 编号继续恢复为整数；
			// heart_beat 的 server_time/last_send_time 参与客户端延迟计算，全部保留 Float。
			keepFloat := p.Method == "heart_beat" || (p.Method == "sync_server_time" && i == 0)
			var err error
			args[i], err = wireValue(value, keepFloat)
			if err != nil {
				g.logf("推送 %s 参数编码失败: %v", p.Method, err)
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		params, err := mobileproto.EncodeMsgpackArgs(args)
		if err != nil {
			g.logf("推送 %s 参数编码失败: %v", p.Method, err)
			continue
		}
		// 默认不将完整热修正文、卡背包等编码为巨量十六进制日志。
		if os.Getenv("HS_GAME_WIRE_TRACE") == "1" {
			g.logf("推送编码 %s target=%s 参数 %x", p.Method, p.Target, params)
		} else if p.Method != "heart_beat" {
			g.logf("推送 %s target=%s oid=%x 字节=%d", p.Method, p.Target, id, len(params))
		}
		out = append(out, mobileproto.Frame{Command: mobileproto.CmdPushEntity,
			Payload: mobileproto.EncodeEntityMessage(id, p.Method, params, true)})
	}
	return out
}

// 保留服务器构造的嵌套 ObjectId；先整份 JSON 化会把对象变成文本，令 camp/player 索引失配。
func wireValue(value any, keepFloat bool) (any, error) {
	switch v := value.(type) {
	case mobileproto.ObjectID:
		return v, nil
	case []byte:
		return v, nil
	case game.ObjectID:
		raw, err := hex.DecodeString(string(v))
		if err != nil || len(raw) != 12 {
			return nil, errors.New("推送角色标识无效")
		}
		var id mobileproto.ObjectID
		copy(id[:], raw)
		return id, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			converted, err := wireValue(item, keepFloat)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	case mobileproto.Map:
		// 保序键值 map（键可为 ObjectID）；逐对转换，不能 JSON 化。
		out := make(mobileproto.Map, len(v))
		for i, pair := range v {
			key, err := wireValue(pair.Key, keepFloat)
			if err != nil {
				return nil, err
			}
			value, err := wireValue(pair.Value, keepFloat)
			if err != nil {
				return nil, err
			}
			out[i] = mobileproto.Pair{Key: key, Value: value}
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			converted, err := wireValue(item, keepFloat)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	default:
		// 直接遍历有类型容器，JSON中间层会把整数/对象键改成字符串，
		// 还会让struct内的ObjectID丢掉Ext42。标量继续沿既有签名处理。
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
				if rv.IsNil() {
					return nil, nil
				}
				rv = rv.Elem()
			}
			if rv.Type() == reflect.TypeOf(game.ObjectID("")) || rv.Type() == reflect.TypeOf(mobileproto.ObjectID{}) {
				return wireValue(rv.Interface(), keepFloat)
			}
			if rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array || rv.Kind() == reflect.Struct {
				if _, marshal := value.(json.Marshaler); !marshal {
					return wireContainer(rv, keepFloat)
				}
			}
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return rv.Int(), nil
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if rv.Uint() > 1<<63-1 {
					return nil, errors.New("原生推送整数超出Int64")
				}
				return int64(rv.Uint()), nil
			}
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var out any
		if err = json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		if !keepFloat {
			out = jsonInts(out)
		}
		return out, nil
	}
}

// wireContainer只转换结构，不推断普通UUID字符串为对象；对象字段由业务明确指定。
func wireContainer(v reflect.Value, keepFloat bool) (any, error) {
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			item, err := wireValue(v.Index(i).Interface(), keepFloat)
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Map:
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
		plainKeys := v.Type().Key().Kind() == reflect.String && v.Type().Key() != reflect.TypeOf(game.ObjectID(""))
		plain := map[string]any{}
		pairs := mobileproto.Map{}
		for _, raw := range keys {
			item, err := wireValue(v.MapIndex(raw).Interface(), keepFloat)
			if err != nil {
				return nil, err
			}
			if plainKeys {
				plain[raw.String()] = item
				continue
			}
			key, err := wireValue(raw.Interface(), false)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, mobileproto.Pair{Key: key, Value: item})
		}
		if plainKeys {
			return plain, nil
		}
		return pairs, nil
	case reflect.Struct:
		out := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" || field.Tag.Get("wire") == "-" {
				continue
			}
			parts := strings.Split(field.Tag.Get("json"), ",")
			name := parts[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			omit := false
			for _, option := range parts[1:] {
				if option == "omitempty" {
					omit = true
				}
			}
			if omit && v.Field(i).IsZero() {
				continue
			}
			item, err := wireValue(v.Field(i).Interface(), keepFloat)
			if err != nil {
				return nil, err
			}
			out[name] = item
		}
		return out, nil
	}
	return nil, errors.New("原生推送容器类型不支持")
}

// normalizeArgs 把 msgpack→JSON 结果规范为实参列表：
// 数字键 map（{"0":a0,"1":a1}，客户端 call_server_method 参数序号）展开为数组，
// 其余形态作为单参透传。
func normalizeArgs(raw []byte) ([]json.RawMessage, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		if list == nil {
			list = []json.RawMessage{}
		}
		return list, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err == nil {
		if len(doc) == 0 {
			return []json.RawMessage{}, nil
		}
		// 客户端 call_server_method 键为 "_0"/"_1"…（实测 bin2='_0'）；兼容纯数字键。
		for _, prefix := range []string{"_", ""} {
			out := make([]json.RawMessage, 0, len(doc))
			for i := 0; ; i++ {
				v, ok := doc[prefix+fmt.Sprint(i)]
				if !ok {
					break
				}
				out = append(out, v)
			}
			if len(out) == len(doc) && len(doc) > 0 {
				return out, nil
			}
		}
	}
	return []json.RawMessage{raw}, nil
}

// zlibCompress 是标准 zlib 压缩（保留给一次性压缩场景）。
func zlibCompress(payload []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(payload)
	_ = zw.Close()
	return buf.Bytes()
}

// jsonInts 递归把 JSON 泛型化产生的整数值 float64 还原为 int64
// （客户端 int() 无法解析 "10001.0"，实机 on_get_all_avatars 证实）。
func jsonInts(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case []any:
		for i := range x {
			x[i] = jsonInts(x[i])
		}
		return x
	case map[string]any:
		for k := range x {
			x[k] = jsonInts(x[k])
		}
		return x
	}
	return v
}
