package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestLogLeagueRecommendationReadsRealPublicStateWithoutWrites(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	ctx := context.Background()
	owner := hexOf(selectedOID(cs[0]))
	peer := hexOf(selectedOID(cs[1]))
	_, err := store.UpdateSocial(ctx, []string{owner}, func(v map[string]*Avatar) error {
		v[owner].Progress.AvatarLevel = 10
		v[owner].Progress.Materials[11] = Material{ID: 11, Count: 5000, Total: 5000}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Handle(ctx, cs[0], "create_new_league", rawArgs("日志学会", "公开简介", 4401)); err != nil {
		t.Fatal(err)
	}
	id := cs[0].SelectedAvatarUnsafe().Progress.LeagueID
	_, err = store.UpdateSocial(ctx, []string{owner}, func(v map[string]*Avatar) error {
		v[owner].Progress.OwnedLeagues[id].PrivateNotice = "不能公开"
		v[owner].Progress.OwnedLeagues[id].Applications[peer] = 123
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := socialSaved(t, store, cs[0]).Progress
	out, err := s.Handle(ctx, cs[2], "get_recommend_leagues", nil)
	reply := remainingFindPush(out, "on_get_recommend_leagues")
	if err != nil || reply == nil {
		t.Fatal(err, out)
	}
	list := reply.Args[0].([]any)
	if len(list) != 1 {
		t.Fatal("未返回真实可加入学会", list)
	}
	info := list[0].(map[string]any)
	if info["trial_score"] != 0 {
		t.Fatal("原生试炼分数不是整数")
	}
	for _, key := range []string{"boss_challenge_hps", "boss_max_hps"} {
		if _, ok := info[key].(map[int]int); !ok {
			t.Fatal("原生血量不是字典", key)
		}
	}
	fixture, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("原生学会推荐夹具=%s", fixture)
	if info["eid"] != ObjectID(id) || info["member_count"] != 1 || info["public_notice"] != "公开简介" || info["private_notice"] != "" || reflect.ValueOf(info["apply_map"]).Len() != 0 {
		t.Fatal("推荐内容不正确或暴露内部资料", info)
	}
	if !reflect.DeepEqual(before, socialSaved(t, store, cs[0]).Progress) {
		t.Fatal("推荐查询改变存档")
	}
	cs[2].hostnum = 2
	out, err = s.recommendLeagues(ctx, cs[2], nil)
	if err != nil || len(out[0].Args[0].([]any)) != 0 {
		t.Fatal("推荐泄露他服学会", err, out)
	}
}
