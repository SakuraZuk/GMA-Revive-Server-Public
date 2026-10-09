package mobileproto

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestObjectIDExtension(t *testing.T) {
	id := ObjectID{0x6a, 0xc3, 0x29, 0x10, 0xb2, 0xe8, 0x6a, 0xcd, 0x79, 0xb1, 0x3d, 0xf0}
	encoded := EncodeMsgpackValue(nil, []any{id})
	if !bytes.Equal(encoded[:4], []byte{0x91, 0xc7, 12, 42}) {
		t.Fatal("ObjectId 扩展编号或长度错误")
	}
	body, err := DecodeMsgpackToJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	if json.Unmarshal(body, &values) != nil || values[0] != "6ac32910b2e86acd79b13df0" {
		t.Fatal("标识解码丢失", string(body))
	}
	for i := 0; i < len(encoded); i++ {
		if _, err = DecodeMsgpackToJSON(encoded[:i]); err == nil {
			t.Fatalf("截断扩展未拒绝 offset=%d", i)
		}
	}
}

func TestObjectIDExtensionAcceptsAndroidHexText(t *testing.T) {
	encoded := append([]byte{0x91, 0xc7, 24, 42}, []byte("6ac32910b2e86acd79b13df0")...)
	body, err := DecodeMsgpackToJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	if json.Unmarshal(body, &values) != nil || len(values) != 1 || values[0] != "6ac32910b2e86acd79b13df0" {
		t.Fatal("Android 十六进制 ObjectId 解码丢失", string(body))
	}

	bad := append([]byte{0x91, 0xc7, 24, 42}, []byte("6ac32910b2e86acd79b13dfx")...)
	if _, err = DecodeMsgpackToJSON(bad); err == nil {
		t.Fatal("非法十六进制 ObjectId 未拒绝")
	}
}
