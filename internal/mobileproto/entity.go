// EntityMessage / EntityInfo / ConnectServer 等消息的 protobuf 编解码。
// 字段号来自 out/proto_desc/client_gate.proto 与 common.proto；
// method 采用 md5=方法名原文 + index=0 的免表方案（out/gate-protocol-spec.md 第三节）。
package mobileproto

// IGateService 方法序（客户端→服务端命令号）。
const (
	CmdSeedRequest   = 0
	CmdSessionKey    = 1
	CmdConnectServer = 2
	CmdEntityMessage = 3
)

// IGateClient 方法序（服务端→客户端命令号）。
const (
	CmdSeedReply     = 0
	CmdSessionKeyOK  = 1
	CmdConnectReply  = 2
	CmdCreateEntity  = 3
	CmdDestroyEntity = 4
	CmdPushEntity    = 5
)

// EntityMessage 字段：1=routes 2=id 3=method(Md5OrIndex) 4=parameters 5=reliable 6=localid。
type EntityMessage struct {
	ID         []byte
	MethodMD5  []byte
	HasIndex   bool
	MethodIdx  int32
	Parameters []byte
	Reliable   bool
	LocalID    int32
}

// MethodName 还原方法名：raw（方法名原文）优先；raw 为空再查静态表。
// 实机证实客户端 salt 派生的动态 index 会与静态表撞号，必须 raw 优先（2026-10-05 联调）。
func (m *EntityMessage) MethodName() string {
	if len(m.MethodMD5) > 0 {
		return string(m.MethodMD5)
	}
	if m.HasIndex && m.MethodIdx > 0 {
		return StaticRPCIndex[int(m.MethodIdx)]
	}
	return ""
}

func DecodeEntityMessage(payload []byte) (EntityMessage, error) {
	fields, err := Fields(payload)
	if err != nil {
		return EntityMessage{}, err
	}
	var m EntityMessage
	for _, f := range fields {
		switch f.Number {
		case 2:
			m.ID = f.Bytes
		case 3:
			sub, err := Fields(f.Bytes)
			if err != nil {
				return EntityMessage{}, err
			}
			for _, s := range sub {
				switch s.Number {
				case 1:
					m.MethodMD5 = s.Bytes
				case 2:
					m.HasIndex = true
					m.MethodIdx = int32(int32(s.Varint))
				}
			}
		case 4:
			m.Parameters = f.Bytes
		case 5:
			m.Reliable = f.Varint != 0
		case 6:
			m.LocalID = int32(f.Varint)
		}
	}
	return m, nil
}

// EncodeEntityMessage 生成 S→C 推送负载：method=名字原文+index=0。
func EncodeEntityMessage(id []byte, method string, parameters []byte, reliable bool) []byte {
	var out []byte
	if id != nil {
		out = AppendField(out, 2, WireBytes, id)
	}
	md5orindex := AppendField(nil, 1, WireBytes, []byte(method))
	out = AppendField(out, 3, WireBytes, md5orindex)
	if parameters != nil {
		out = AppendField(out, 4, WireBytes, parameters)
	}
	if reliable {
		out = AppendVarintField(out, 5, 1)
	}
	return out
}

// EntityInfo 字段：1=routes 2=type(Md5OrIndex) 3=id 4=info。
func EncodeCreateEntity(typeName string, id []byte, info []byte) []byte {
	var out []byte
	md5orindex := AppendField(nil, 1, WireBytes, []byte(typeName))
	out = AppendField(out, 2, WireBytes, md5orindex)
	if id != nil {
		out = AppendField(out, 3, WireBytes, id)
	}
	if info != nil {
		out = AppendField(out, 4, WireBytes, info)
	}
	return out
}

// ConnectServerRequest 字段：1=routes 2=type 3=deviceid 4=entityid 5=authmsg。
type ConnectRequest struct {
	Type     int32
	DeviceID []byte
	EntityID []byte
	AuthMsg  []byte
}

func DecodeConnectRequest(payload []byte) (ConnectRequest, error) {
	fields, err := Fields(payload)
	if err != nil {
		return ConnectRequest{}, err
	}
	var r ConnectRequest
	for _, f := range fields {
		switch f.Number {
		case 2:
			r.Type = int32(f.Varint)
		case 3:
			r.DeviceID = f.Bytes
		case 4:
			r.EntityID = f.Bytes
		case 5:
			r.AuthMsg = f.Bytes
		}
	}
	return r, nil
}

// EncodeConnectReply 字段：1=routes 2=type 3=entityid 4=extramsg。
func EncodeConnectReply(replyType int32, entityID []byte) []byte {
	var out []byte
	out = AppendVarintField(out, 2, uint64(uint32(replyType)))
	if entityID != nil {
		out = AppendField(out, 3, WireBytes, entityID)
	}
	return out
}

// EncryptString 字段 1=encryptstr；SessionKey 字段 2=session_key 3=seed。
func DecodeEncryptString(payload []byte) ([]byte, error) {
	fields, err := Fields(payload)
	if err != nil {
		return nil, err
	}
	if f, ok := First(fields, 1); ok {
		return f.Bytes, nil
	}
	return nil, nil
}

func DecodeSessionKey(payload []byte) (key []byte, seed int64, err error) {
	fields, err := Fields(payload)
	if err != nil {
		return nil, 0, err
	}
	for _, f := range fields {
		switch f.Number {
		case 2:
			key = f.Bytes
		case 3:
			seed = int64(f.Varint)
		}
	}
	return key, seed, nil
}

// StaticRPCIndex 是客户端 mobilecommon.py 硬编码的 STATIC_RPC_METHOD2INDEX
// （out/gate-protocol-spec.md，31 项战斗/AOI 高频方法；登录链路不使用）。
var StaticRPCIndex = map[int]string{
	1: "sync_souls_pos_v2", 2: "sync_monster_cmds", 3: "sync_props",
	4: "sync_space_props", 5: "sync_stop_move_cmd", 6: "on_create_space_entity",
	7: "notify_use_skill_or_normal_atk", 8: "sync_other_add_state", 9: "sync_other_remove_state",
	10: "on_destroy_space_entity", 11: "face_to", 12: "sync_refresh_state",
	13: "on_create_sub_mfs", 14: "sync_own_props", 15: "on_show_simple_hit_and_calc",
	16: "on_create_sub_es", 17: "on_es_created_success", 18: "on_ping_cb_space",
	19: "sync_other_skill_cd", 20: "update_other_client_cash", 21: "update_other_client_exp",
	22: "sync_other_dead", 23: "update_other_client_total_cash", 24: "on_show_calc_result",
	25: "on_sync_sub_es_destroy", 26: "sync_combat_props", 27: "on_create_fly_sfx",
	28: "sync_start_move_cmd", 29: "on_show_hit_and_calc", 30: "sync_add_state",
	31: "sync_remove_state",
}
