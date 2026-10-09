package game

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// 保留各RPC原有资产/回调顺序断言，同时严格核对额外成就投影的值和回调前时序。
func checkedCorePushes(t *testing.T, pushes []Push, p Progress) []Push {
	t.Helper()
	core := []Push{}
	seen := map[string]bool{}
	callbackSeen := false
	for _, output := range pushes {
		if output.Method == "call_client_callback" {
			callbackSeen = true
		}
		if output.Target == "Avatar" && output.Method == "client_prop_changed" && len(output.Args) == 1 {
			property, ok := output.Args[0].([]any)
			if ok && len(property) == 2 {
				name, _ := property[0].(string)
				if name == "achves" || name == "achv_value" {
					if callbackSeen || seen[name] {
						t.Fatal("成就投影重复或落在原生回调之后", name)
					}
					var want any = achievementProperties(p)
					if name == "achv_value" {
						if !seen["achves"] {
							t.Fatal("成就分数早于成就状态")
						}
						want = achievementPoints(p)
					}
					if !reflect.DeepEqual(property[1], want) {
						t.Fatal("成就推送与已提交进度不一致", name)
					}
					seen[name] = true
					continue
				}
			}
		}
		core = append(core, output)
	}
	if seen["achves"] != seen["achv_value"] {
		t.Fatal("成就状态和分数未成对推送")
	}
	return core
}

func TestAchievementFreshStateDoesNotInventEvents(t *testing.T) {
	now := time.Unix(1791360000, 0)
	p := NewProgress(1, now)
	if len(p.Achievements) != len(androidAchievements.Rules) || p.AchievementLoginDay != "" || achievementTargetCount(p, 207001, 207001) != 0 || achievementTargetCount(p, 209001, 1008) != 0 {
		t.Fatal("新角色成就目录缺失或补造了登录、胜利、消费事件")
	}
	before := CloneProgress(p)
	reconcileAchievementState(&p, 1, now)
	if !reflect.DeepEqual(before, CloneProgress(p)) {
		t.Fatal("无关首次操作再次初始化已投影的成就")
	}
}

func TestAchievementLegacyFirstEventAndFailureAtomic(t *testing.T) {
	s, c := newSyncPVPTestService()
	ctx := context.Background()
	store := s.Accounts.(*FixtureAccounts)
	if _, err := store.UpdateProgress(ctx, selectedOID(c), func(p *Progress) error { p.Achievements = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		recordAchievementCompetitiveWin(p, 5, s.Now())
		return errors.New("实际事件后拒绝事务")
	}); err == nil {
		t.Fatal("失败事务被接受")
	}
	before, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || len(before.Achievements) != 0 {
		t.Fatal("失败操作初始化或推进旧成就", err)
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error { recordAchievementCompetitiveWin(p, 5, s.Now()); return nil }); err != nil {
		t.Fatal(err)
	}
	after, err := store.UpdateProgress(ctx, selectedOID(c), func(*Progress) error { return nil })
	if err != nil || len(after.Achievements) != len(androidAchievements.Rules) || achievementTargetCount(after, 207001, 207001) != 1 {
		t.Fatal("旧空存档首次真实事件丢失", err)
	}
}
