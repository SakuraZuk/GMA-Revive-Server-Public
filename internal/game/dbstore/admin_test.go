package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"hs-server/internal/game"
	"math"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresPlayerAdminAtomicProfileAssetsAndReceipts(t *testing.T) {
	store := testStore(t)
	now := time.Now()
	info := game.ClientInfo{Account: "管理资产验收", Password: "pw", Hostnum: 1}
	svc, c, av := repairPlayer(t, store, info, &now)
	defer svc.Detach(c)
	token := strings.Repeat("t", 32)
	h := game.AdminHandler(svc, token)
	req := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "真实库管理001", "operator": "测试员", "reason": "并发及事务", "operation": "material_add", "material_id": 12, "amount": 5}
	body, _ := json.Marshal(req)
	call := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/admin/player/update", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:19001"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	before, err := store.AdminPlayer(context.Background(), av.OID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- call(body).Code }()
	}
	wg.Wait()
	close(results)
	created := 0
	for code := range results {
		if code == 201 {
			created++
		} else if code != 200 {
			t.Fatal(code)
		}
	}
	if created != 1 {
		t.Fatal("并发收据重复发放")
	}
	after, err := New(store.pool).AdminPlayer(context.Background(), av.OID)
	if err != nil || after.Progress.Materials[12].Count != before.Progress.Materials[12].Count+5 || len(after.Progress.PlayerAdminReceipts) != 1 {
		t.Fatal("管理资产收据未同事务持久", err)
	}
	_, err = store.UpdateProgress(context.Background(), av.OID, func(p *game.Progress) error {
		p.Materials[12] = game.Material{ID: 12, Count: math.MaxInt64, Total: math.MaxInt64}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	req["receipt"] = "真实库管理002"
	body, _ = json.Marshal(req)
	if call(body).Code != 422 {
		t.Fatal("容量拒绝失败")
	}
	after, err = New(store.pool).AdminPlayer(context.Background(), av.OID)
	if err != nil || len(after.Progress.PlayerAdminReceipts) != 1 || after.Progress.Materials[12].Count != math.MaxInt64 {
		t.Fatal("失败写入收据或修改资产", err)
	}
	req = map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "receipt": "真实库资料001", "operator": "测试员", "reason": "资料JSONB原子持久", "operation": "profile", "nickname": "新管理馆主", "level": 10, "gender": 2}
	body, _ = json.Marshal(req)
	if w := call(body); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err = New(store.pool).AdminPlayer(context.Background(), av.OID)
	if err != nil || after.Info.Nickname != "新管理馆主" || after.Info.Level != 10 || after.Gender != 2 || len(after.Progress.PlayerAdminReceipts) != 2 {
		t.Fatal("资料与审计没有原子提交", err)
	}
}
