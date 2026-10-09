package game

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/hotfix"
)

func TestRemainingActivityMikuOldNullJSONActualLoginAndColdService(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, shanghaiZone)
	activitySchedulesForTest(t, now)
	var catalog hotfix.Catalog
	if err := catalog.Load(filepath.Join("..", "..", "deploy", "data", "hotfix.json")); err != nil {
		t.Fatal("加载实际热修目录失败", err)
	}
	for _, test := range []struct{ name, raw string }{
		{"全部旧null容器", `{"map_infos":null,"achv_info":null,"tasks":null}`},
		{"已有地图的旧null嵌套容器", `{"map_infos":{"1":{"map_id":1,"nodes":null,"treasure_state":null,"explore_coins":null,"story_list":null,"handbook_info":{"handbook_items":null},"server_completed_runs":1}},"achv_info":null,"tasks":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			info := ClientInfo{Account: "初音旧档" + test.name, Password: "pw", Hostnum: 10001}
			store := NewFixtureAccounts(nil)
			id, err := store.Register(ctx, info)
			if err != nil {
				t.Fatal(err)
			}
			oid := id.Avatars[0].OID
			if _, err = store.UpdateProgress(ctx, oid, func(p *Progress) error {
				p.AvatarLevel = 40
				p.ClearedDungeons = []int{601, 610}
				var legacy MikuState
				if err := json.Unmarshal([]byte(test.raw), &legacy); err != nil {
					return err
				}
				p.Activities.Miku = &legacy
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err = store.UpdateSocial(ctx, []string{hexOf(oid)}, func(rows map[string]*Avatar) error { rows[hexOf(oid)].Progress.AvatarLevel = 40; return nil }); err != nil {
				t.Fatal(err)
			}
			login := func(accounts *FixtureAccounts) (*Service, *Connection) {
				svc := New(accounts, &catalog)
				svc.Now = func() time.Time { return now }
				c := NewConnection()
				c.SetDeviceID("旧档登录回归")
				pushes, err := svc.Handle(ctx, c, "quick_login", rawArgs(info))
				if err != nil || len(pushes) != 3 || pushes[0].Method != "login_result" || pushes[0].Args[0] != RetSuccess || c.Phase() != Authenticated {
					t.Fatal("实际Handle旧档登录失败", err, pushes)
				}
				if _, err = svc.BecomePlayer(c); err != nil {
					t.Fatal(err)
				}
				pushes, err = svc.Handle(ctx, c, "set_reconnect_auth_msg", rawArgs("旧档绑定凭据"))
				if err != nil || len(pushes) != 1 || pushes[0].Method != "on_refresh_login" {
					t.Fatal("实际Handle登录收尾失败", err, pushes)
				}
				return svc, c
			}
			svc, c := login(store)
			p := c.SelectedAvatarUnsafe().Progress
			runtime := CloneProgress(p)
			w := ensureMiku(&runtime) // 可选空映射omitempty冷存后允许nil；实际读写入口必须恢复可写映射。
			if w.Maps == nil || w.Achievements == nil || w.Tasks == nil || w.Visits == nil || w.Battles == nil || w.Gifts == nil || len(w.Achievements) == 0 {
				t.Fatal("生产迁移未建立可写空容器/原成就")
			}
			if m := w.Maps[1]; m != nil {
				if m.Nodes == nil || m.Treasure == nil || m.Coins == nil || m.Stories == nil || m.Handbook.Items == nil || m.ReceiptMaterials == nil || m.Bonus == nil || m.CompletedRuns != 1 || m.SurpriseClaimed {
					t.Fatal("嵌套迁移丢完成状态或补造惊喜", m)
				}
			}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var restored Progress
			if err = json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			av := c.SelectedAvatarUnsafe()
			av.Progress = restored
			svc.Detach(c)
			coldStore := NewFixtureAccounts(map[string]FixtureAccount{info.Account: {Password: info.Password, Avatars: []Avatar{av}}})
			cold, coldC := login(coldStore)
			defer cold.Detach(coldC)
			after := coldC.SelectedAvatarUnsafe().Progress
			if !reflect.DeepEqual(p.Activities.Miku, after.Activities.Miku) || !reflect.DeepEqual(p.Materials, after.Materials) || !reflect.DeepEqual(p.Cards, after.Cards) || !reflect.DeepEqual(p.Runes, after.Runes) {
				t.Fatal("JSON冷重建再次迁移改活动收据或资产")
			}
		})
	}
}
