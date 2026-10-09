package dbstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hs-server/internal/game"
	"hs-server/internal/hotfix"
)

// 活动PG夹具使用项目实际热修发布目录，不能把nil Catalog当作成功登录。
func pgActivityService(t *testing.T, store *Store, now func() time.Time) *game.Service {
	t.Helper()
	catalog := &hotfix.Catalog{}
	if err := catalog.Load(filepath.Join("..", "..", "..", "deploy", "data", "hotfix.json")); err != nil {
		t.Fatal("加载实际热修目录失败", err)
	}
	svc := game.New(store, catalog)
	svc.Now = now
	return svc
}

// 与gate相同：真实账号鉴权成功，确认角色与热修回包后才创建玩家会话。
// 每次重新登录都建立新Connection，禁止复用Playing连接绕过状态检查。
func pgActivityLogin(t *testing.T, svc *game.Service, info game.ClientInfo) *game.Connection {
	t.Helper()
	ctx := context.Background()
	c := game.NewConnection()
	c.SetDeviceID("活动PG设备-" + info.Account)
	if c.Phase() != game.Connected {
		t.Fatal("活动PG新连接初始状态错误")
	}
	pushes, err := svc.Handle(ctx, c, "quick_login", pgSocialArgs(info))
	if err != nil || len(pushes) != 3 || pushes[0].Method != "login_result" || pushes[0].Args[0] != game.RetSuccess || pushes[1].Method != "on_get_all_avatars" || pushes[2].Method != "on_hotfix_when_login" || c.Phase() != game.Authenticated {
		t.Fatal("活动PG真实账号登录未完成", err, c.Phase())
	}
	avatar, ok := c.SelectedAvatar()
	if !ok || len(avatar.OID) != 12 || avatar.Account != info.Account || avatar.Hostnum != info.Hostnum {
		t.Fatal("活动PG登录角色绑定无效")
	}
	hf := svc.Hotfix.Query(info.HotfixIndex)
	if pushes[2].Args[0] != hf.Script || pushes[2].Args[1] != hf.Index {
		t.Fatal("活动PG登录热修不是目录实际快照")
	}
	if _, err = svc.BecomePlayer(c); err != nil || c.Phase() != game.Playing {
		t.Fatal("活动PG角色成为玩家失败", err, c.Phase())
	}
	pushes, err = svc.Handle(ctx, c, "set_reconnect_auth_msg", pgSocialArgs("活动PG角色绑定完成"))
	if err != nil || len(pushes) == 0 || c.Phase() != game.Playing {
		t.Fatal("活动PG角色绑定后的登录收尾失败", err, pushes)
	}
	refreshes, starts, results := 0, 0, 0
	for _, item := range pushes {
		switch item.Method {
		case "on_refresh_login":
			refreshes++
		case "start_server_battle_ok":
			starts++
		case "battle_result":
			results++
		}
	}
	// 新进程没有旧battle实体：完成态进入大厅，资产和收据已经持久。
	// 未完成态仍优先重建battle，不能先启动下一条引导。
	if avatar.Progress.Battle == nil {
		if refreshes != 1 || starts != 0 || results != 0 {
			t.Fatal("无战斗登录必须唯一正常收尾", pushes)
		}
	} else if avatar.Progress.Battle.Finished {
		if results != 0 || refreshes != 1 || starts != 0 {
			t.Fatal("完成态冷登录必须进入大厅，不向空实体补旧结果", pushes)
		}
	} else if starts != 1 || refreshes != 0 || results != 0 {
		t.Fatal("未完成态冷登录必须先恢复战斗", pushes)
	}
	t.Cleanup(func() { svc.Detach(c) })
	return c
}
