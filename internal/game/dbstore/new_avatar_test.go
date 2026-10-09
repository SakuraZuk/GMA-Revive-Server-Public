package dbstore

import (
	"context"
	"encoding/json"
	"testing"

	"hs-server/internal/game"
)

func TestPostgresNewAvatarAllHeroesSwitchColdLogin(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	oldInfo := game.ClientInfo{Account: "原初始英雄账号", Password: "测试密码", Hostnum: 10001}
	old, err := s.Register(ctx, oldInfo)
	if err != nil || len(old.Avatars[0].Progress.Cards) != 1 {
		t.Fatal("关闭开关的注册失败", err)
	}
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "1")
	info := game.ClientInfo{Account: "新角色全英雄账号", Password: "测试密码", Hostnum: 10001}
	first, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	p := first.Avatars[0].Progress
	if len(p.Cards) != 82 || len(p.ObtainedCardIDs) != 82 {
		t.Fatal("本版82位可用图鉴英雄未完整保存", len(p.Cards))
	}
	before, _ := json.Marshal(p.Cards)
	// 新存储连接冷登录不重复赠送；开关开启也不修改旧角色。
	again, err := New(s.pool).QuickLogin(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(again.Avatars[0].Progress.Cards)
	if string(before) != string(after) {
		t.Fatal("重登重建或重复赠送英雄")
	}
	legacy, err := s.QuickLogin(ctx, oldInfo)
	if err != nil || len(legacy.Avatars[0].Progress.Cards) != 1 {
		t.Fatal("旧角色被补发", err)
	}
	// 旧存档缺进度时沿用普通恢复，不把恢复当作新建角色。
	if _, err = s.pool.Exec(ctx, `DELETE FROM avatar_progress WHERE avatar_oid=$1`, old.Avatars[0].OID); err != nil {
		t.Fatal(err)
	}
	legacy, err = s.QuickLogin(ctx, oldInfo)
	if err != nil || len(legacy.Avatars[0].Progress.Cards) != 1 {
		t.Fatal("旧进度恢复错误赠送", err)
	}
	info.Hostnum++
	newHost, err := s.QuickLogin(ctx, info)
	if err != nil || len(newHost.Avatars[1].Progress.Cards) != 82 {
		t.Fatal("同账号新服建角未赠送", err)
	}
	t.Setenv("HS_NEW_AVATAR_ALL_HEROES", "0")
	again, err = s.QuickLogin(ctx, info)
	if err != nil || len(again.Avatars[1].Progress.Cards) != 82 {
		t.Fatal("关闭开关回收已赠英雄", err)
	}
}
