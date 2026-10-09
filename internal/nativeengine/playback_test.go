package nativeengine

import (
	"encoding/json"
	"testing"
)

func TestNativePlaybackRequiresOrderedCommandThenLaterBoundaryAndAuthorityWinner(t *testing.T) {
	p := Checkpoint{Generation: 1}
	if err := p.Observe(1, 2, "started", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	command := []any{"use_skill", []any{160101, "target"}}
	if err := p.Track("caster", command, ""); err != nil {
		t.Fatal(err)
	}
	if p.Ready() {
		t.Fatal("命令刚投递就通过播放屏障")
	}
	before, _ := json.Marshal(p)
	for _, call := range []struct {
		gen, seq int64
		eid      string
		command  any
	}{{0, 3, "caster", command}, {1, 2, "caster", command}, {1, 3, "other", command}, {1, 3, "caster", []any{"move_to", []any{[]int{0, 0, 0}}}}} {
		if err := p.Observe(call.gen, call.seq, "command", call.eid, call.command, nil); err == nil {
			t.Fatal("旧世代/旧序号/乱序播放被接受", call)
		}
		after, _ := json.Marshal(p)
		if string(after) != string(before) {
			t.Fatal("拒绝播放确认修改了持久游标")
		}
	}
	// 客户端原生命令含攻击站位，普通技能确认只核对权威技能和目标。
	if err := p.Observe(1, 3, "command", "caster", []any{"use_skill", []any{160101, "target", []int{1, -1, 0}, []int{2, -2, 0}}}, nil); err != nil {
		t.Fatal(err)
	}
	if p.Ready() {
		t.Fatal("log_command被当成动画与原生续回合完成")
	}
	if err := p.Observe(1, 4, "round_end", "other", nil, nil); err != nil {
		t.Fatal(err)
	}
	if p.Ready() {
		t.Fatal("错误回合单位越过播放屏障")
	}
	if err := p.Observe(1, 5, "input", "caster", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !p.Ready() {
		t.Fatal("稍后原生输入边界没有确认播放")
	}
	if err := p.Observe(1, 6, "result", "", nil, []string{"client_forged_winner"}); err != nil {
		t.Fatal(err)
	}
	if p.TerminalMatches([]string{"server_winner"}, 1) {
		t.Fatal("客户端末态选择了服务端胜方")
	}
	if err := p.Observe(1, 7, "result", "", nil, []string{"server_winner"}); err != nil {
		t.Fatal(err)
	}
	if !p.TerminalMatches([]string{"server_winner"}, 1) {
		t.Fatal("双方同一权威结果没有满足末态屏障")
	}
	// 原生后续命令必须废弃此前可能抢跑的客户端result。
	if err := p.Track("caster", []any{"idle", []any{}}, ""); err != nil {
		t.Fatal(err)
	}
	if p.ResultCandidate != nil || p.TerminalMatches([]string{"server_winner"}, 1) {
		t.Fatal("新增权威动作仍沿用提前结束结果")
	}
}

func TestNativePlaybackSupportResumesOriginalFighterAndPersistsGeneration(t *testing.T) {
	p := Checkpoint{Generation: 9}
	if err := p.Observe(9, 2, "started", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	command := []any{"use_support_skill", []any{220201, "target", []int{1, -1, 0}, []int{2, -2, 0}}}
	if err := p.Track("support", command, "fighter"); err != nil {
		t.Fatal(err)
	}
	if err := p.Observe(9, 3, "command", "support", command, nil); err != nil {
		t.Fatal(err)
	}
	for i, event := range []struct{ kind, eid string }{{"round_end", "support"}, {"turn", "other"}, {"round_end", "fighter"}} {
		if err := p.Observe(9, int64(4+i), event.kind, event.eid, nil, nil); err != nil {
			t.Fatal(err)
		}
		if p.Ready() {
			t.Fatal("援护播放没有恢复原战斗单位却通过屏障", event)
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var cold Checkpoint
	if err = json.Unmarshal(raw, &cold); err != nil {
		t.Fatal(err)
	}
	if err = cold.Observe(9, 7, "input", "fighter", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !cold.Ready() {
		t.Fatal("冷读取后原战斗单位输入未解除援护屏障")
	}
	if err = cold.Observe(9, 8, "result", "", nil, []string{"left"}); err != nil {
		t.Fatal(err)
	}
	cold.Generation = 10
	if cold.TerminalMatches([]string{"left"}, 1) {
		t.Fatal("旧场景末态被新恢复世代接受")
	}
	if err = cold.Observe(9, 9, "result", "", nil, []string{"left"}); err == nil {
		t.Fatal("旧世代结果推进了新场景")
	}
}
