package game

import (
	"context"
	"log"
)

// 调用方已持有连接锁；真实鉴权收尾与Tick重试复用同一持久化和恢复链。
func (s *Service) finishLogin(ctx context.Context, c *Connection) (result []Push, resultErr error) {
	defer func() {
		if resultErr != nil {
			c.nextLoginFinishRetry = s.Now().UnixMilli() + 1000
			log.Printf("登录收尾延后 uid=%d，将由Tick重试：%v", c.SelectedAvatarUnsafe().UID, resultErr)
		}
	}()
	if err := s.saveReconnect(ctx, c, c.reconnectAuth); err != nil {
		return nil, err
	}
	// 客户端在 Avatar.on_login_success 收尾时发本上行（dis/2034D84C.asm 420-435），
	// 是 player 已完成角色绑定、可安全执行 on_refresh_login 的信号。
	// 过早在 BecomePlayer 推送会因 player 仍绑定 Account 抛 AttributeError（2026-10-05 15:33 实机）。
	// 每连接只回一次：on_refresh_login 内部会再次触发 on_login_success 并再发本上行，防循环。
	if !c.refreshLoginSent {
		assistPushes := []Push{}
		// Avatar已绑定，先恢复原生剧情桶，再启动地图/活动剧情检查。
		if len(c.SelectedAvatarUnsafe().Progress.PlayedStorylines) > 0 {
			assistPushes = append(assistPushes, storylinePush(c.SelectedAvatarUnsafe().Progress))
		}
		if c.SelectedAvatarUnsafe().Progress.UnlockSystems["assist"] > 0 {
			if _, supported := s.Accounts.(SocialAccounts); supported {
				var err error
				assistPushes, err = s.friendAssistRPC(ctx, c, "refresh_assist_use_times", nil)
				if err != nil {
					return nil, err
				}
			}
		}
		// 恢复事务失败不能提前吞掉后续登录收尾请求。
		defer func() {
			if resultErr == nil {
				c.refreshLoginSent = true
			}
		}()
		// 新进程重登时客户端本地已没有 battle 对象，原生 net_delay 不会再主动
		// 调 client_need_recover_battle。若服务端仍保存未结或待补发结果的战斗，
		// 必须先走同一恢复事务，不能让 on_refresh_login 直接启动下一条引导副本。
		if av := c.SelectedAvatarUnsafe(); av.Progress.Battle != nil {
			if av.Progress.Battle.Finished && av.Progress.Battle.NativeSolo == nil && av.Progress.Social.HumanRoom == nil && av.Progress.SyncPvpMatch == nil && (av.Progress.AsyncPvp.Match == nil || av.Progress.AsyncPvp.Match.UUID != av.Progress.Battle.UUID) {
				// 新进程没有结算所需的原生战斗实体，不能直接推battle_result。
				// 已提交资产在create_entity中同步，保留原收据供有本地battle的
				// 显式恢复补发；冷登录只执行当前教学/主城加载，不重发奖励。
				return append(assistPushes, push("Avatar", "on_refresh_login")), nil
			}
			unfinishedBattle := !av.Progress.Battle.Finished
			recoveryPushes, err := s.clientNeedRecoverBattle(ctx, c, nil)
			if err != nil {
				return nil, err
			}
			if !unfinishedBattle {
				return append(assistPushes, recoveryPushes...), nil
			}
			// 恢复推送先创建battle，再执行原生登录收尾以移除登录视频/遮罩。
			// runtime revision 4会识别已下发的start_server_battle_ok，不重复请求恢复。
			return append(append(assistPushes, recoveryPushes...), push("Avatar", "on_refresh_login")), nil
		}
		// 引导通过 create_entity.guide_tasks 初始化；条件通知是普通本地方法，不可推 RPC。
		return append(assistPushes, push("Avatar", "on_refresh_login")), nil
	}
	return []Push{}, nil
}
