package main

import (
	"bytes"
	"hs-server/internal/game"
	"hs-server/internal/mobileproto"
	"testing"
)

func TestGuideTipBinarySurvivesActualGateConversion(t *testing.T) {
	message := []byte("引导任务不存在或未激活")
	converted, err := wireValue([]any{false, message}, false)
	if err != nil {
		t.Fatal(err)
	}
	args := converted.([]any)
	if !bytes.Equal(args[1].([]byte), message) {
		t.Fatal("拒绝提示字节被JSON化")
	}
	wire := mobileproto.EncodeMsgpackValue(nil, converted)
	if len(wire) < 4 || wire[0] != 0x92 || wire[1] != 0xc2 || wire[2] != 0xc4 {
		t.Fatal("原生回调失败提示缺少bin类型", wire)
	}
}

func TestNestedBattleObjectIDs(t *testing.T) {
	value, err := wireValue([]any{[]any{game.ObjectID("6ac32910b2e86acd79b13df0")}, 10001, int64(123), 10001}, false)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mobileproto.EncodeMsgpackValue(nil, value)
	if !bytes.Contains(encoded, []byte{0xc7, 12, 42, 0x6a, 0xc3, 0x29, 0x10}) {
		t.Fatal("嵌套 ObjectId 被变成字符串")
	}
	if _, err = wireValue(game.ObjectID("bad"), false); err == nil {
		t.Fatal("错误标识获准")
	}
}

// 真实线缆回归：嵌套struct/整数键/对象键均不能经过JSON丢类型。
func TestWireTypedSettlementContainers(t *testing.T) {
	id := game.ObjectID("6ac32910b2e86acd79b13df0")
	type card struct {
		UUID   game.ObjectID `json:"uuid"`
		Level  int           `json:"level"`
		Hidden string        `wire:"-"`
	}
	source := map[string]any{
		"materials": map[int]int64{2: 9007199254740993},
		"runes":     map[game.ObjectID]card{id: {UUID: id, Level: 1, Hidden: "内部账本"}},
		"cards":     []card{{UUID: id, Level: 2}},
	}
	converted, err := wireValue(source, false)
	if err != nil {
		t.Fatal(err)
	}
	box := converted.(map[string]any)
	materials, ok := box["materials"].(mobileproto.Map)
	if !ok || len(materials) != 1 || materials[0].Key != int64(2) || materials[0].Value != int64(9007199254740993) {
		t.Fatal("材料整数键或大整数精度丢失", box)
	}
	runes, ok := box["runes"].(mobileproto.Map)
	if !ok || len(runes) != 1 {
		t.Fatal("对象键被改为字符串", box)
	}
	if _, ok := runes[0].Key.(mobileproto.ObjectID); !ok {
		t.Fatal("契印键没有Ext42", runes)
	}
	c := runes[0].Value.(map[string]any)
	if _, ok := c["uuid"].(mobileproto.ObjectID); !ok {
		t.Fatal("实例UUID没有Ext42", c)
	}
	if _, exists := c["Hidden"]; exists {
		t.Fatal("内部账本泄漏", c)
	}
	encoded := mobileproto.EncodeMsgpackValue(nil, converted)
	if bytes.Count(encoded, []byte{0xc7, 12, 42}) != 3 {
		t.Fatal("真实编码未保留三个对象", encoded)
	}
	if _, err = mobileproto.DecodeMsgpackToJSON(encoded); err != nil {
		t.Fatal("真实编码无法回读", err)
	}
	value, err := wireValue(map[int]int64{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mobileproto.EncodeMsgpackValue(nil, value)) != 1 || mobileproto.EncodeMsgpackValue(nil, value)[0] != 0x80 {
		t.Fatal("空整数字典不能变数组/null")
	}
}
