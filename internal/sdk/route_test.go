package sdk

import "testing"

func TestSelectRouteByVersion(t *testing.T) {
	game := "192.0.2.10:9000"
	hotfix := "192.0.2.20:443"
	if got := SelectRoute("1.0.125", "1.0.125,1.0.126", game, hotfix, false); got.Mode != "hotfix" || got.Address != hotfix {
		t.Fatalf("命中版本未走热更服：%+v", got)
	}
	if got := SelectRoute("1.0.127", "1.0.125,1.0.126", game, hotfix, false); got.Mode != "game" || got.Address != game {
		t.Fatalf("未命中版本未走游戏服：%+v", got)
	}
	if got := SelectRoute("", "", game, hotfix, true); got.Mode != "hotfix" {
		t.Fatal("强制热更开关未生效")
	}
}
