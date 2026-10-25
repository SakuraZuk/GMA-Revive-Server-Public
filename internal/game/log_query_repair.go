package game

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
)

// 日志和原生调用点共同确认的轻量入口；不把埋点当作付费或领奖凭据。
func (s *Service) logQueryRepair(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("该请求需要已登录角色")
	}
	switch method {
	case "set_focus_target":
		var target string
		var kind int
		if len(args) != 2 || json.Unmarshal(args[0], &target) != nil || len(target) > 128 || json.Unmarshal(args[1], &kind) != nil {
			return nil, errors.New("聚焦目标参数无效")
		}
		p := c.SelectedAvatarUnsafe().Progress
		if p.Battle == nil || p.Battle.Finished {
			return nil, nil // 结束后的鼠标/触摸事件不再改变旧模型。
		}
		if p.Battle.NativeSolo != nil || humanRoomActive(p.Social.HumanRoom) {
			return nil, errors.New("权威竞技聚焦尚未接入原生输入日志")
		}
		// shadow_battle没有on_set_focus_target；原生player_input需要操作者eid。
		var targetID any = target
		if raw, err := hex.DecodeString(target); err == nil && len(raw) == 12 {
			targetID = ObjectID(target) // 非空实体引用恢复ExtType42；空字符串仍表示取消。
		}
		return []Push{battleSync("set_focus_target", ObjectID(hex.EncodeToString(c.SelectedAvatarUnsafe().OID)), targetID, kind)}, nil
	case "request_chat_channel":
		if len(args) != 0 {
			return nil, errors.New("聊天频道请求不带参数")
		}
		channel := c.SelectedAvatarUnsafe().Progress.ChatChannel
		if channel < 1 || channel > maxChatChannel {
			channel = 1
		}
		raw, _ := json.Marshal(channel)
		return s.changeChatChannel(ctx, c, []json.RawMessage{raw})
	case "query_phone_info":
		if len(args) != 0 {
			return nil, errors.New("手机信息查询不带参数")
		}
		// 本服账号没有第三方手机号绑定；空信息明确表示未绑定，不代做认证。
		return []Push{push("Avatar", "on_query_phone_info", RetSuccess, map[string]any{"login_type": 0, "mobile_num": ""})}, nil
	case "close_asyn_pvp_ui":
		if len(args) != 0 {
			return nil, errors.New("关闭竞技页面不带参数")
		}
		return nil, nil
	case "home_enter_quit_log", "share_click_log", "notice_share", "receive_package_check":
		counts := map[string]int{"home_enter_quit_log": 3, "share_click_log": 2, "notice_share": 0, "receive_package_check": 1}
		if len(args) != counts[method] {
			return nil, errors.New("客户端上报参数数量无效")
		}
		for _, arg := range args {
			if len(arg) > 4096 || !json.Valid(arg) {
				return nil, errors.New("客户端上报参数无效或过长")
			}
		}
		// 包名与分享上报只作诊断；不记录内容或签发认证，更不据此增加资产。
		log.Printf("客户端行为上报 method=%s 参数数=%d", method, len(args))
		return nil, nil
	}
	return nil, errors.New("未知日志修补入口")
}
