package game

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// 原表同桌活动首段剧情的两个角色选项；只核验真实业务与冷存档，不代替剧情渲染。
func TestExamOriginalCharacterChoicesAndColdPersistence(t *testing.T) {
	const file = "story/gaokao/story/gk1_1_1b"
	const node = "choose_character_53"
	for _, choice := range []struct {
		option   string
		material int
	}{{"A", 209011}, {"B", 209012}} {
		t.Run(choice.option, func(t *testing.T) {
			ctx := context.Background()
			a := NewFixtureAccounts(nil)
			s := New(a, nil)
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, shanghaiZone)
			s.Now = func() time.Time { return now }
			activitySchedulesForTest(t, now)
			c, av := newBattleConnection(t, ctx, a, s)
			if err := s.updateProgress(ctx, c, func(p *Progress) error {
				p.AvatarLevel = 60
				p.ClearedDungeons = append(p.ClearedDungeons, 510, 601)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			out, err := s.Handle(ctx, c, "receive_storyline_bonus", rawArgs(501, file, node, choice.option))
			if err != nil || len(out) < 2 {
				t.Fatal("角色选项未完成业务回包", err, out)
			}
			callback := out[len(out)-1]
			result, ok := callback.Args[1].([]any)
			if callback.Method != "call_client_callback" || callback.Args[0] != int64(501) || !ok || len(result) != 2 || result[0] != int64(RetSuccess) {
				t.Fatal("原版双参回调未成功", callback)
			}
			stored, err := a.UpdateProgress(ctx, av.OID, func(p *Progress) error { return nil })
			key := strings.ReplaceAll(file+"_"+node, ".", "")
			if err != nil || stored.Activities.Exam == nil || stored.Activities.Exam.Storyline[key] != choice.option || stored.Materials[choice.material].Count != 1 {
				t.Fatal("角色选择或真实标识材料未冷存", err)
			}
			before := CloneProgress(stored)
			for _, retry := range []string{choice.option, map[string]string{"A": "B", "B": "A"}[choice.option]} {
				out, err = s.Handle(ctx, c, "receive_storyline_bonus", rawArgs(502, file, node, retry))
				if err != nil || len(out) == 0 || out[len(out)-1].Args[1].([]any)[0] == int64(RetSuccess) || !reflect.DeepEqual(before, c.SelectedAvatarUnsafe().Progress) {
					t.Fatal("重选角色重复发放或拒绝改变资产", err, out)
				}
			}
		})
	}
}
