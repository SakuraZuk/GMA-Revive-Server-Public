package game

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestSyncPvpAndroidRobotCardsHaveSkillsAndRunes(t *testing.T) {
	for _, template := range syncPvpCatalog.Tables.RobotCards {
		wire := robotCardWire(template)
		if wire["dress"].(int) <= 0 {
			t.Fatalf("机器人默认外观缺失 %d", template.ID)
		}
		skills, ok := wire["skill_mgr"].([]any)
		if !ok || len(skills) != len(syncPvpCatalog.Tables.CardSkills[strconv.Itoa(template.CardID)].Skills) || len(skills) == 0 {
			t.Fatalf("机器人技能快照缺失 %d", template.ID)
		}
		embed := wire["embed_runes"].(map[string]any)
		if len(embed) != len(template.Runes) {
			t.Fatalf("机器人契印模板未完整物化 %d", template.ID)
		}
		for _, item := range embed {
			r := item.(map[string]any)
			if !validObjectID(r["uuid"].(string)) || r["star"].(int) <= 0 || r["pos"].(int) <= 0 || r["suit"].(int) <= 0 {
				t.Fatalf("机器人契印字段不合法 %d", template.ID)
			}
		}
	}
}
func TestSyncPvpPeriodRewardOnceAndClockRollback(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	old := s.Now()
	if err := s.updateProgress(ctx, cs[0], func(p *Progress) error { return ensureSyncPvpPeriod(p, old) }); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return old.Add(8 * 24 * time.Hour) }
	pushes, err := s.receiveSyncPvpSeason(ctx, cs[0], nil)
	if err != nil || len(pushes) == 0 {
		t.Fatal(err)
	}
	p := socialSaved(t, store, cs[0]).Progress
	if p.Materials[11].Count != 100 {
		t.Fatal("赛季奖励不是Android分段奖", p.Materials[11])
	}
	if _, err = s.receiveSyncPvpSeason(ctx, cs[0], nil); err == nil {
		t.Fatal("赛季奖励重复领取未拒绝")
	}
	s.Now = func() time.Time { return old }
	if _, err = s.querySyncPVPSeason(ctx, cs[0], nil); err == nil {
		t.Fatal("赛季时间回拨未拒绝")
	}
}
