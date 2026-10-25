package dbstore

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hs-server/internal/game"
)

func TestPostgresMainlineAutoEnhanceHistoricalLoginAndColdDuplicate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	info := game.ClientInfo{Account: "主线自动补完实库", Password: "pw", Hostnum: 1}
	id, err := s.Register(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	av, err := s.SetNicknameGender(ctx, id.Avatars[0].OID, "补完验收", 1)
	if err != nil {
		t.Fatal(err)
	}
	// 原版chapter_dict第一章所有非支线/活动地点；包含末尾纯剧情714。
	chapter := []int{501, 502, 503, 504, 505, 506, 507, 508, 509, 510, 511, 512, 513, 514,
		601, 602, 603, 604, 605, 606, 607, 608, 609, 610, 611, 612, 613, 614, 615, 616, 617, 618, 619,
		701, 702, 703, 704, 705, 706, 707, 708, 709, 710, 711, 712, 713, 714}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.Cards = []game.Card{{UUID: "00112233445566778899aabb", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1},
			{UUID: "00112233445566778899aacc", CardID: 4401, Level: 1, Grade: 1, SupportSkillLevel: 1}}
		p.ClearedDungeons = append([]int(nil), chapter[:len(chapter)-1]...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	login := func() game.Progress {
		t.Helper()
		fresh := New(s.pool)
		pgActivityLogin(t, pgActivityService(t, fresh, func() time.Time { return now }), info)
		reloaded, err := fresh.QuickLogin(ctx, info)
		if err != nil {
			t.Fatal(err)
		}
		return reloaded.Avatars[0].Progress
	}
	if p := login(); p.Cards[0].EnhanceCount != 0 {
		t.Fatal("第一章未完整通关却补完")
	}
	if _, err := s.UpdateProgress(ctx, av.OID, func(p *game.Progress) error {
		p.ClearedDungeons = append([]int(nil), chapter...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := login()
	if first.Cards[0].EnhanceCount != 2 || first.Cards[0].SkillEnhanceCount != 1 || first.Cards[1].EnhanceCount != 0 {
		t.Fatal("历史存档补齐原表阶段失败", first.Cards)
	}
	second := login()
	if !reflect.DeepEqual(first.Cards, second.Cards) || !reflect.DeepEqual(first.Materials, second.Materials) {
		t.Fatal("新服务冷登录重复发补完或消耗材料")
	}
}
