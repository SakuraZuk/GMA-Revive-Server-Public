package nativeengine

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNativeClickAndPlaybackUseProvenOwnerEntitiesAndBothSupportPositions(t *testing.T) {
	c := entityCases(t)[0]
	owners := [2]string{"left", "right"}
	mapping := BindEntities(c.Native, c.Client, false, nil, nil)
	frame, err := DeriveCoordinateFrame(c.Native, c.Client, mapping)
	if err != nil {
		t.Fatal(err)
	}
	state := State{Units: c.Native, CurrentInputEID: "n1", CurrentMaster: "left", AwaitingPlayer: true}
	before, _ := json.Marshal(state)
	click, err := MapClick("move_to", []any{"81", []int{4, -1, -3}}, state, owners, "left", mapping, frame)
	if err != nil || !reflect.DeepEqual(click["args"], []any{"n1", [3]int{1, -1, 0}}) {
		t.Fatal("反射棋盘点击未转入原生坐标", click, err)
	}
	for _, bad := range []struct {
		id   string
		args []any
	}{{"right", []any{"81", []int{4, -1, -3}}}, {"left", []any{"73", []int{4, -1, -3}}}, {"left", []any{"未映射", []int{4, -1, -3}}}, {"left", []any{"81", []int{1, 1, 1}}}} {
		if _, err := MapClick("move_to", bad.args, state, owners, bad.id, mapping, frame); err == nil {
			t.Fatal("无权、未映射或坏坐标点击被接受", bad)
		}
	}
	if _, err := MapClick("use_skill", []any{"ck_monster", "73", 210301, "81"}, state, owners, "left", mapping, frame); err == nil {
		t.Fatal("对方施法者被鉴权玩家控制")
	}
	if _, err := MapClick("use_skill", []any{"ck_monster", "81", 160101, "未知目标"}, state, owners, "left", mapping, frame); err == nil {
		t.Fatal("未映射目标直接送入原生")
	}
	event := Event{Kind: "sync_command", Data: map[string]any{"args": []any{"left", "n1", []any{"use_skill", []any{160101, "n2", []int{0, 0, 0}, []int{1, -1, 0}}}}}}
	own, err := MapPlayback(event, state, owners, "left", mapping, frame)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := MapPlayback(event, state, owners, "right", mapping, frame)
	if err != nil {
		t.Fatal(err)
	}
	if own.Method != "revival_do_command" || peer.Method != "revival_do_peer_command" || peer.Master != "left" || !reflect.DeepEqual(peer.Args, []any{"use_skill", []any{"ck_monster", "81", 160101, "73"}}) {
		t.Fatal("权威技能广播形态不符", own, peer)
	}
	support := Event{Kind: "sync_command", Data: map[string]any{"round_eid": "n1", "args": []any{"left", "ns", []any{"use_support_skill", []any{220201, "n2", []int{1, -1, 0}, []int{2, -2, 0}}}}}}
	play, err := MapPlayback(support, state, owners, "right", mapping, frame)
	want := []any{"81", "5", []any{"use_support_skill", []any{220201, "73", [3]int{4, -1, -3}, [3]int{3, 0, -3}}}}
	if err != nil || play.Method != "revival_play_support_command" || !reflect.DeepEqual(play.Args, want) {
		t.Fatal("援护丢失原回合或两个原生位置", play, err)
	}
	support.Data["round_eid"] = "n2"
	if _, err := MapPlayback(support, state, owners, "right", mapping, frame); err == nil {
		t.Fatal("援护被中断回合与控制者不符仍广播")
	}
	event.Data["args"] = []any{"left", "n1", []any{"stun", []any{}}}
	play, err = MapPlayback(event, state, owners, "right", mapping, frame)
	if err != nil || play.Method != "revival_play_native_command" {
		t.Fatal("原生强制控制被再次思考替换", play, err)
	}
	event.Data["args"] = []any{"left", "n1", []any{"stun", []any{1}}}
	if _, err := MapPlayback(event, state, owners, "right", mapping, frame); err == nil {
		t.Fatal("带参数强制控制被接受")
	}
	after, _ := json.Marshal(state)
	if !bytes.Equal(before, after) {
		t.Fatal("点击与播放转换改写了冻结原生快照")
	}
}

func TestNativeActualManualDuelCommandsTranslateForBothClients(t *testing.T) {
	config := nativeTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := NewAuthority(config, testResourceHash)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	journal, raw, err := a.Begin(ctx, journalDuelStart(t, ctx, config))
	if err != nil {
		t.Fatal(err)
	}
	update, err := ParseUpdate(raw)
	if err != nil {
		t.Fatal(err)
	}
	owners := [2]string{"000000000000000000000001", "000000000000000000000002"}
	mapping := map[string]string{}
	frame := &CoordinateFrame{Sign: -1, Offset: [3]int{5, -2, -3}, AnchorCount: 2}
	commands, windows := 0, 0
	for n := 0; n < 500 && update.Result == nil; n++ {
		operation := "drive"
		if update.State.AwaitingPlayer {
			operation = "timeout"
			windows++
		}
		journal, raw, err = a.Advance(ctx, journal, map[string]any{"operation": operation})
		if err != nil {
			t.Fatal(err)
		}
		update, err = ParseUpdate(raw)
		if err != nil {
			t.Fatal(err)
		}
		// 客户端EID和棋盘为显式转换夹具；输出命令来自实际原生引擎。
		for _, unit := range update.State.Units {
			eid := identityEID(unit["eid"])
			mapping[eid] = "client-" + eid
		}
		for _, event := range update.Events {
			if event.Kind != "sync_command" {
				continue
			}
			for _, recipient := range owners {
				play, err := MapPlayback(event, update.State, owners, recipient, mapping, frame)
				if err != nil {
					t.Fatal("真实原生命令无法保持两端播放语义", err, event)
				}
				if play != nil {
					commands++
				}
			}
		}
	}
	if update.Result == nil || commands == 0 || windows == 0 {
		t.Fatal("真实双手动PVP未经过输入窗口/两端命令并结束", commands, windows)
	}
	if len(update.Result.Winners) != 1 || (update.Result.Winners[0] != owners[0] && update.Result.Winners[0] != owners[1]) {
		t.Fatal("原生最终胜方不属于冻结房间", update.Result)
	}
	a.Close()
	b, err := NewAuthority(config, testResourceHash)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	cold, err := b.Restore(ctx, journalClone(t, journal))
	if err != nil || !bytes.Equal(cold, raw) {
		t.Fatal("完整原生终态冷重建后分歧", err)
	}
	t.Logf("真实原生手动超时对局：%d个输入窗口，%d次接收端命令转换，%d个已校验日志项", windows, commands, len(journal.Entries))
}
