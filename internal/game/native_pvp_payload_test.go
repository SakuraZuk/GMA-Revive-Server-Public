package game

import (
	"bytes"
	"context"
	"encoding/json"
	"hs-server/internal/nativeengine"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePvpPayloadUsesFrozenCardsAndRejectsCrossOwnerUUID(t *testing.T) {
	_, _, _, r := humanTestPair(t, true)
	before := mustJSON(r)
	metadata, cards, err := nativePvpPayload(r)
	if err != nil || len(cards) == 0 || metadata["avatar_id"] != r.IDs[0] || metadata["enemy_id"] != r.IDs[1] || !bytes.Equal(before, mustJSON(r)) {
		t.Fatal("原生权威冻结源失配或改变原房间", err)
	}
	peer := r.Players[r.IDs[1]]
	peer.Cards = r.Players[r.IDs[0]].Cards
	peer.Layout = r.Players[r.IDs[0]].Layout
	r.Players[r.IDs[1]] = peer
	if _, _, err = nativePvpPayload(r); err == nil {
		t.Fatal("双方同一个卡UUID被接受")
	}
}

func TestNativePvpActualGoFrozenRoomStartsOriginalEngine(t *testing.T) {
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON2")
	if python == "" {
		t.Skip("隔离Python2真实引擎只由专项显式启用")
	}
	_, _, _, r := humanTestPair(t, true)
	metadata, roster, err := nativePvpPayload(r)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	engine, err := nativeengine.Open(ctx, nativeengine.Config{Python: python, Worker: filepath.Join(root, "battle_native_worker.py"), Directory: filepath.Join(root, "runtime/native_engine")})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	update, err := engine.Request(ctx, map[string]any{"operation": "start", "metadata": metadata, "roster": roster, "seed": r.Seed, "auto": false})
	if err != nil {
		t.Fatal("实际Go冻结卡、槽位、技能、潜质/契印未被原生接受", err)
	}
	var state struct {
		State struct {
			Units []map[string]any `json:"units"`
		} `json:"state"`
		Result any `json:"result"`
	}
	if err = json.Unmarshal(update, &state); err != nil || len(state.State.Units) == 0 || state.Result != nil {
		t.Fatal("Go房间原生启动状态错误", err)
	}
}
