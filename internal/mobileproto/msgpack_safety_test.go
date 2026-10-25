package mobileproto

import (
	"bytes"
	"testing"
)

func TestTupleKeyMatchesNativeCustomDictAndNeverEmitsArrayMapKey(t *testing.T) {
	if got := TupleKey(1, 2); got != "(1, 2)" {
		t.Fatalf("二元Tuple键格式错误: %q", got)
	}
	if got := TupleKey(1, 0, 0, 0, 0); got != "(1, 0, 0, 0, 0)" {
		t.Fatalf("五元Tuple键格式错误: %q", got)
	}
	encoded := EncodeMsgpackValue(nil, Map{{Key: []any{1, 2}, Value: true}})
	if bytes.Contains(encoded, []byte{0x92, 0x01, 0x02}) {
		t.Fatalf("复合键仍以msgpack数组发送: %x", encoded)
	}
	if !bytes.Contains(encoded, []byte("(1, 2)")) {
		t.Fatalf("复合键未按原版字符串发送: %x", encoded)
	}
	if _, err := DecodeMsgpackToJSON(encoded); err != nil {
		t.Fatalf("修复后的复合键无法解码: %v", err)
	}
}

func TestMsgpackTruncationHugeCountsAndDepthAreRejected(t *testing.T) {
	valid := EncodeMsgpackMap(map[string]any{"_0": []any{1.25, int64(999999), "中文", map[string]any{"_1": true}}})
	if _, err := DecodeMsgpackToJSON(valid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(valid); i++ {
		if _, err := DecodeMsgpackToJSON(valid[:i]); err == nil {
			t.Fatalf("截断位置 %d 未被拒绝", i)
		}
	}
	for _, bad := range [][]byte{{0xdd, 255, 255, 255, 255}, {0xdf, 255, 255, 255, 255}, {0xcb, 0}, {0xd3}, {0xd9}} {
		if _, err := DecodeMsgpackToJSON(bad); err == nil {
			t.Fatal("非法包未被拒绝")
		}
	}
	deep := []byte{0}
	for i := 0; i < 65; i++ {
		deep = append([]byte{0x91}, deep...)
	}
	if _, err := DecodeMsgpackToJSON(deep); err == nil {
		t.Fatal("嵌套深度没有限制")
	}
}
