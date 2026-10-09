// Package mobileproto 的 BSON 子集编解码。
// 依据 out/gate-protocol-spec.md：客户端默认 proto='BSON'（gate_client_config），
// EntityMessage.parameters 为 BSON 文档；数组在线格式上就是 {"0":v0,"1":v1...} 文档。
// 仅实现 common.proto 实际会出现的类型：double/string/document/array/binary/
// boolean/datetime/null/int32/int64（UTC datetime）。
package mobileproto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

var errBSON = errors.New("bson: 非法数据")

// DecodeBSONDoc 解析一个完整 BSON 文档为有序键值。
// 返回 map[string]any；数组返回键 "0","1",… 的文档（与客户端 bson.BSON.decode 行为一致）。
func DecodeBSONDoc(data []byte) (map[string]any, error) {
	v, rest, err := decodeDoc(data)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("bson: 文档后有 %d 字节残留", len(rest))
	}
	return v, nil
}

// DecodeBSONToJSON 把 BSON 文档转成 JSON 字节，供 internal/game 的 json.RawMessage 业务层使用。
// 数组文档（键恰为 "0","1",… 且连续）输出 JSON 数组，与 msgpack 参数列表对齐。
func DecodeBSONToJSON(data []byte) ([]byte, error) {
	doc, err := DecodeBSONDoc(data)
	if err != nil {
		return nil, err
	}
	return marshalJSON(arrayAware(doc))
}

// arrayAware 把 BSON 数组文档（键 "0".."n-1" 连续且覆盖全部键）还原为切片。
func arrayAware(doc map[string]any) any {
	n := len(doc)
	if n == 0 {
		return doc
	}
	out := make([]any, n)
	for i := 0; i < n; i++ {
		v, ok := doc[fmt.Sprint(i)]
		if !ok {
			return doc
		}
		out[i] = v
	}
	return out
}

func decodeDoc(data []byte) (map[string]any, []byte, error) {
	if len(data) < 5 {
		return nil, nil, errBSON
	}
	total := int(binary.LittleEndian.Uint32(data[:4]))
	if total < 5 || total > len(data) {
		return nil, nil, errBSON
	}
	body := data[4 : total-1]
	if data[total-1] != 0 {
		return nil, nil, errBSON
	}
	rest := data[total:]
	out := make(map[string]any)
	for i := 0; i < len(body); {
		t := body[i]
		i++
		key, n, err := decodeCString(body[i:])
		if err != nil {
			return nil, nil, err
		}
		i += n
		v, n, err := decodeValue(t, body[i:])
		if err != nil {
			return nil, nil, err
		}
		i += n
		out[key] = v
	}
	return out, rest, nil
}

func decodeCString(data []byte) (string, int, error) {
	for i := 0; i < len(data); i++ {
		if data[i] == 0 {
			return string(data[:i]), i + 1, nil
		}
	}
	return "", 0, errBSON
}

func decodeValue(t byte, data []byte) (any, int, error) {
	switch t {
	case 0x01: // double
		if len(data) < 8 {
			return nil, 0, errBSON
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(data[:8])), 8, nil
	case 0x02: // string
		if len(data) < 4 {
			return nil, 0, errBSON
		}
		n := int(binary.LittleEndian.Uint32(data[:4]))
		if n <= 0 || 4+n > len(data) || data[4+n-1] != 0 {
			return nil, 0, errBSON
		}
		return string(data[4 : 4+n-1]), 4 + n, nil
	case 0x03, 0x04: // document / array
		doc, rest, err := decodeDoc(data)
		if err != nil {
			return nil, 0, err
		}
		if t == 0x04 {
			return arrayAware(doc), len(data) - len(rest), nil
		}
		return doc, len(data) - len(rest), nil
	case 0x05: // binary
		if len(data) < 5 {
			return nil, 0, errBSON
		}
		n := int(binary.LittleEndian.Uint32(data[:4]))
		if n < 0 || 5+n > len(data) {
			return nil, 0, errBSON
		}
		return append([]byte(nil), data[5:5+n]...), 5 + n, nil
	case 0x08: // boolean
		if len(data) < 1 {
			return nil, 0, errBSON
		}
		return data[0] != 0, 1, nil
	case 0x09: // UTC datetime（毫秒）
		if len(data) < 8 {
			return nil, 0, errBSON
		}
		return int64(binary.LittleEndian.Uint64(data[:8])), 8, nil
	case 0x0A: // null
		return nil, 0, nil
	case 0x10: // int32
		if len(data) < 4 {
			return nil, 0, errBSON
		}
		return int64(int32(binary.LittleEndian.Uint32(data[:4]))), 4, nil
	case 0x12: // int64
		if len(data) < 8 {
			return nil, 0, errBSON
		}
		return int64(binary.LittleEndian.Uint64(data[:8])), 8, nil
	default:
		return nil, 0, fmt.Errorf("bson: 未实现类型 0x%02x", t)
	}
}

// EncodeBSONArgs 把参数列表编码为 BSON 数组文档（客户端 method(args) 单参调用契约）。
func EncodeBSONArgs(args []any) ([]byte, error) {
	doc := make(map[string]any, len(args))
	for i, a := range args {
		doc[fmt.Sprint(i)] = a
	}
	return encodeDoc(doc)
}

// EncodeBSONDoc 编码单个 BSON 文档（create_entity 的 info 字段）。
func EncodeBSONDoc(doc map[string]any) ([]byte, error) {
	return encodeDoc(doc)
}

func encodeDoc(doc map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var body []byte
	for _, k := range keys {
		body = appendValue(body, k, doc[k])
	}
	out := make([]byte, 4, 4+len(body)+1)
	out = append(out, body...)
	out = append(out, 0)
	binary.LittleEndian.PutUint32(out[:4], uint32(len(out)))
	return out, nil
}

func cstring(s string) []byte {
	return append(append([]byte(nil), s...), 0)
}

// appendValue 写入完整 BSON 元素：类型字节 + cstring 键 + 值。
func appendValue(dst []byte, key string, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(append(dst, 0x0A), cstring(key)...)
	case bool:
		t := byte(0)
		if x {
			t = 1
		}
		return append(append(dst, 0x08), append(cstring(key), t)...)
	case int:
		return appendInteger(dst, key, int64(x))
	case int32:
		dst = append(dst, 0x10)
		dst = append(dst, cstring(key)...)
		return append(dst, byte(x), byte(x>>8), byte(x>>16), byte(x>>24))
	case int64:
		return appendInteger(dst, key, x)
	case float64:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
		dst = append(dst, 0x01)
		dst = append(dst, cstring(key)...)
		return append(dst, b[:]...)
	case string:
		s := cstring(x)
		dst = append(dst, 0x02)
		dst = append(dst, cstring(key)...)
		dst = append(dst, byte(len(s)), byte(len(s)>>8), byte(len(s)>>16), byte(len(s)>>24))
		return append(dst, s...)
	case []byte:
		dst = append(dst, 0x05)
		dst = append(dst, cstring(key)...)
		dst = append(dst, byte(len(x)), byte(len(x)>>8), byte(len(x)>>16), byte(len(x)>>24), 0x00)
		return append(dst, x...)
	case []any:
		doc := make(map[string]any, len(x))
		for i, e := range x {
			doc[fmt.Sprint(i)] = e
		}
		enc, _ := encodeDoc(doc)
		dst = append(dst, 0x04)
		dst = append(dst, cstring(key)...)
		return append(dst, enc...)
	case map[string]any:
		enc, _ := encodeDoc(x)
		dst = append(dst, 0x03)
		dst = append(dst, cstring(key)...)
		return append(dst, enc...)
	default:
		// 其余类型按字符串兜底，与客户端 msgpackext 的 repr 行为一致。
		return appendValue(dst, key, fmt.Sprint(x))
	}
}

// appendInteger 按 BSON 惯例优先 int32，超范围用 int64。
func appendInteger(dst []byte, key string, x int64) []byte {
	if x == int64(int32(x)) {
		dst = append(dst, 0x10)
		dst = append(dst, cstring(key)...)
		return append(dst, byte(x), byte(x>>8), byte(x>>16), byte(x>>24))
	}
	dst = append(dst, 0x12)
	dst = append(dst, cstring(key)...)
	return append(dst, byte(x), byte(x>>8), byte(x>>16), byte(x>>24), byte(x>>32), byte(x>>40), byte(x>>48), byte(x>>56))
}

func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
