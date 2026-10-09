package mobileproto

import "testing"

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
