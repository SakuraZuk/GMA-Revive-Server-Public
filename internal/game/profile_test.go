package game

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNamingContract(t *testing.T) {
	for _, name := range []string{"落徒彦", "艾丽丝", "Alice9", "カリン", "测试_1"} {
		if ValidateNickname(name) != 0 {
			t.Errorf("合法名字拒绝：%s", name)
		}
	}
	for _, name := range []string{"甲", "ABCDEFGHIJ", "测试🙂", "abc def", "测试\n"} {
		if ValidateNickname(name) == 0 {
			t.Errorf("非法名字放行：%s", name)
		}
	}
	accounts := NewFixtureAccounts(nil)
	info := ClientInfo{Account: "创建测试", Password: "pw", Hostnum: 1}
	identity, err := accounts.Register(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	c := NewConnection()
	c.identity = identity
	c.hostnum = 1
	c.phase = Playing
	s := New(accounts, nil)
	args := []json.RawMessage{json.RawMessage(`7`), json.RawMessage(`"落徒彦"`), json.RawMessage(`1`)}
	pushes, err := s.Handle(context.Background(), c, "set_nickname_gender", args)
	if err != nil || len(pushes) != 4 || pushes[3].Method != "call_client_callback" {
		t.Fatalf("字段同步和回调错误：%+v %v", pushes, err)
	}
	av, _ := c.SelectedAvatar()
	if av.Info.Nickname != "落徒彦" || !av.NicknameSet || av.Gender != 1 {
		t.Fatal("连接角色资料未更新")
	}
	repeat, err := s.Handle(context.Background(), c, "set_nickname_gender", args)
	if err != nil || len(repeat) != 4 {
		t.Fatal("相同取名不能重试")
	}
	args[1] = json.RawMessage(`"另一个名字"`)
	pushes, err = s.Handle(context.Background(), c, "set_nickname_gender", args)
	if err != nil || len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != RetNicknameExists {
		t.Fatal("取名接口允许免费改名")
	}
	voiceArgs := []json.RawMessage{json.RawMessage(`8`), json.RawMessage(`1`)}
	for _, method := range []string{"set_global_vo", "set_story_vo"} {
		result, err := s.Handle(context.Background(), c, method, voiceArgs)
		if err != nil || len(result) != 2 || result[1].Args[1].([]any)[0] != true {
			t.Fatal("语音设置契约错误", result, err)
		}
	}
	voiceArgs[1] = json.RawMessage(`0`)
	result, err := s.Handle(context.Background(), c, "set_story_vo", voiceArgs)
	if err != nil || result[0].Args[1].([]any)[0] != false {
		t.Fatal("非法剧情语音获准")
	}
}
