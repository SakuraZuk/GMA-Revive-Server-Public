package game

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpsAdminLoginOnlineNoticeMailAndGacha(t *testing.T) {
	accounts := NewFixtureAccounts(nil)
	s := New(accounts, nil)
	c, av := newBattleConnection(t, context.Background(), accounts, s)
	s.attachPlayer(c)
	defer s.Detach(c)
	handler := AdminHandler(s, strings.Repeat("x", 32))
	call := func(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, reader)
		r.RemoteAddr = "127.0.0.1:19001"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := call(http.MethodPost, "/admin/session", map[string]string{"user": "admin", "password": "wrong"}, nil); got.Code != http.StatusUnauthorized {
		t.Fatal("错误密码仍可登录", got.Code)
	}
	if got := call(http.MethodGet, "/admin/online", nil, nil); got.Code != http.StatusUnauthorized {
		t.Fatal("未登录可读在线", got.Code)
	}
	login := call(http.MethodPost, "/admin/session", map[string]string{"user": "admin", "password": defaultAdminPassword}, nil)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatal("管理员登录失败", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	online := call(http.MethodGet, "/admin/online", nil, cookie)
	var onlineBody struct {
		Count   int         `json:"count"`
		Players []onlineRow `json:"players"`
	}
	if online.Code != http.StatusOK || json.Unmarshal(online.Body.Bytes(), &onlineBody) != nil || onlineBody.Count != 1 || onlineBody.Players[0].UID != av.UID {
		t.Fatal("在线明细不正确", online.Code, online.Body.String())
	}
	if strings.Contains(online.Body.String(), c.deviceID) && c.deviceID != "" {
		t.Fatal("在线明细暴露完整设备号")
	}
	activities := call(http.MethodGet, "/admin/activities", nil, cookie)
	if activities.Code != http.StatusOK || !strings.Contains(activities.Body.String(), "主线剧情") {
		t.Fatal("活动列表不可用", activities.Code, activities.Body.String())
	}
	notice := call(http.MethodPost, "/admin/notices", map[string]any{"title": "维护公告", "body": "今晚短暂停服", "kind": "维护", "pinned": true, "enabled": true}, cookie)
	if notice.Code != http.StatusCreated {
		t.Fatal("公告发布失败", notice.Code, notice.Body.String())
	}
	listed := call(http.MethodGet, "/admin/notices", nil, cookie)
	if !strings.Contains(listed.Body.String(), "维护公告") {
		t.Fatal("公告未列出")
	}
	mail := map[string]any{"receipt": "全服补偿001", "title": "补偿", "content": "请查收", "sender": "阿克夏馆务处", "material_id": 12, "amount": 3, "reason": "运营验收"}
	first := call(http.MethodPost, "/admin/broadcasts", mail, cookie)
	second := call(http.MethodPost, "/admin/broadcasts", mail, cookie)
	if first.Code != http.StatusCreated || second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "重复请求") {
		t.Fatal("全服邮件幂等失败", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	stored, err := accounts.AdminPlayer(context.Background(), av.OID)
	if err != nil || len(stored.Progress.ShortMailInfo) == 0 {
		t.Fatal("全服邮件未入账", err)
	}
	window := map[string]any{"pool_id": 1, "enabled": true, "begin": s.Now().Unix() - 10, "end": s.Now().Unix() + 3600, "note": "常驻池展示"}
	if got := call(http.MethodPost, "/admin/gacha", window, cookie); got.Code != http.StatusCreated {
		t.Fatal("卡池排期失败", got.Code, got.Body.String())
	}
	pools := call(http.MethodGet, "/admin/gacha", nil, cookie)
	if pools.Code != http.StatusOK || !strings.Contains(pools.Body.String(), "排期中") {
		t.Fatal("卡池排期未反映", pools.Body.String())
	}
	if got := call(http.MethodPost, "/admin/logout", map[string]any{}, cookie); got.Code != http.StatusOK {
		t.Fatal("退出失败")
	}
	if got := call(http.MethodGet, "/admin/online", nil, cookie); got.Code != http.StatusUnauthorized {
		t.Fatal("退出后会话仍有效")
	}
}
