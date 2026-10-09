// Package mobileproto 的 msgpack 子集编解码（实机证实客户端 proto=msgpack，
// create_entity/entity_message 的 info/parameters 均为 msgpack，2026-10-05 联调）。
package mobileproto

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// ObjectID 使用原生 ObjectId 扩展编号 42 与十二字节值。
type ObjectID [12]byte

// Pair 是 Map 的单个键值对；键支持任意可编码类型（含 ObjectID）。
type Pair struct {
	Key   any
	Value any
}

// Map 是保序 msgpack map；用于需要非字符串键的容器
// （如客户端按 ObjectId 索引的 last_fighting_cards）。
type Map []Pair

// EncodeMsgpackValue 编码单值；EncodeMsgpackArgs 编码参数列表（数组）。
func EncodeMsgpackValue(dst []byte, v any) []byte {
	switch x := v.(type) {
	case ObjectID:
		return append(append(dst, 0xc7, 12, 42), x[:]...)
	case nil:
		return append(dst, 0xc0)
	case bool:
		if x {
			return append(dst, 0xc3)
		}
		return append(dst, 0xc2)
	case int:
		return appendMsgInt(dst, int64(x))
	case int32:
		return appendMsgInt(dst, int64(x))
	case int64:
		return appendMsgInt(dst, x)
	case float64:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], math.Float64bits(x))
		return append(append(dst, 0xcb), b[:]...)
	case float32:
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], math.Float32bits(x))
		return append(append(dst, 0xca), b[:]...)
	case string:
		dst = appendMsgStrHeader(dst, len(x))
		return append(dst, x...)
	case []byte:
		n := len(x)
		switch {
		case n < 256:
			dst = append(dst, 0xc4, byte(n))
		case n < 65536:
			dst = append(dst, 0xc5, byte(n>>8), byte(n))
		default:
			dst = append(dst, 0xc6, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
		}
		return append(dst, x...)
	case []any:
		dst = appendMsgArrayHeader(dst, len(x))
		for _, e := range x {
			dst = EncodeMsgpackValue(dst, e)
		}
		return dst
	case map[string]any:
		dst = appendMsgMapHeader(dst, len(x))
		for k, val := range x {
			dst = EncodeMsgpackValue(dst, k)
			dst = EncodeMsgpackValue(dst, val)
		}
		return dst
	case Map:
		// 保序键值对 map：键可为 ObjectID 等非字符串类型
		//（客户端 last_fighting_cards 等容器以 ObjectId 为键，字符串键查不到）。
		dst = appendMsgMapHeader(dst, len(x))
		for _, pair := range x {
			dst = EncodeMsgpackValue(dst, pair.Key)
			dst = EncodeMsgpackValue(dst, pair.Value)
		}
		return dst
	default:
		return EncodeMsgpackValue(dst, fmt.Sprint(x))
	}
}

func appendMsgInt(dst []byte, x int64) []byte {
	switch {
	case x >= 0 && x < 128:
		return append(dst, byte(x))
	case x < 0 && x >= -32:
		return append(dst, byte(0xe0|(x+32)))
	case x >= -128 && x < 128:
		return append(dst, 0xd0, byte(x))
	case x >= -32768 && x < 32768:
		return append(dst, 0xd1, byte(x>>8), byte(x))
	case x >= -2147483648 && x < 2147483648:
		return append(dst, 0xd2, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	default:
		return append(dst, 0xd3, byte(x>>56), byte(x>>48), byte(x>>40), byte(x>>32), byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	}
}

func appendMsgStrHeader(dst []byte, n int) []byte {
	switch {
	case n < 32:
		return append(dst, 0xa0|byte(n))
	case n < 256:
		return append(dst, 0xd9, byte(n))
	case n < 65536:
		return append(dst, 0xda, byte(n>>8), byte(n))
	default:
		return append(dst, 0xdb, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
}

func appendMsgArrayHeader(dst []byte, n int) []byte {
	switch {
	case n < 16:
		return append(dst, 0x90|byte(n))
	case n < 65536:
		return append(dst, 0xdc, byte(n>>8), byte(n))
	default:
		return append(dst, 0xdd, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
}

func appendMsgMapHeader(dst []byte, n int) []byte {
	switch {
	case n < 16:
		return append(dst, 0x80|byte(n))
	case n < 65536:
		return append(dst, 0xde, byte(n>>8), byte(n))
	default:
		return append(dst, 0xdf, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
}

// EncodeMsgpackArgs 把参数列表编码为 msgpack map（键="_0"/"_1"…）。
// 客户端 RpcMethod.call 按整数下标取参（rpcdecorator @91-153），其定制 bson/msgpack
// 层以 "_N" 形式承载序号键（实机 quick_login 上行实测键为 bin'_0'）。
func EncodeMsgpackArgs(args []any) ([]byte, error) {
	if args == nil {
		args = []any{}
	}
	doc := make(map[string]any, len(args))
	for i, a := range args {
		doc["_"+strconv.Itoa(i)] = a
	}
	return EncodeMsgpackValue(nil, doc), nil
}

// EncodeMsgpackMap 编码单个文档（create_entity 的 info 字段）。
func EncodeMsgpackMap(doc map[string]any) []byte {
	return EncodeMsgpackValue(nil, doc)
}

var errMsgpack = errors.New("msgpack: 非法数据")

type msgpackCursor struct {
	d     []byte
	i     int
	depth int
}

// DecodeMsgpackToJSON 把 msgpack 值转 JSON 字节供业务层使用。
func DecodeMsgpackToJSON(data []byte) (body []byte, err error) {
	// 截断的标量长度过去会由 take panic，不能让公网坏包终止整个游戏进程。
	defer func() {
		if recover() != nil {
			body = nil
			err = errMsgpack
		}
	}()
	c := &msgpackCursor{d: data}
	v, err := c.value()
	if err != nil {
		return nil, err
	}
	if c.i != len(data) {
		return nil, fmt.Errorf("msgpack: %d 字节残留", len(data)-c.i)
	}
	return json.Marshal(v)
}

func (c *msgpackCursor) value() (any, error) {
	c.depth++
	defer func() { c.depth-- }()
	if c.depth > 64 {
		return nil, errMsgpack
	}
	if c.i >= len(c.d) {
		return nil, errMsgpack
	}
	b := c.d[c.i]
	c.i++
	switch {
	case b <= 0x7f:
		return int64(b), nil
	case b >= 0xe0:
		return int64(int8(b)), nil
	case b >= 0x80 && b <= 0x8f:
		return c.mapN(int(b & 0x0f))
	case b >= 0x90 && b <= 0x9f:
		return c.arrayN(int(b & 0x0f))
	case b >= 0xa0 && b <= 0xbf:
		return c.strN(int(b & 0x1f))
	}
	switch b {
	case 0xc7:
		n := int(c.take(1)[0])
		kind := c.take(1)[0]
		if kind != 42 {
			return nil, errMsgpack
		}
		raw := c.take(n)
		// Android 1.0.128 的 bson_msgpack.msgpackext 使用 str(ObjectId)：
		// 部分 bson 实现得到十二字节原值，客户端当前实现得到二十四位十六进制文本。
		// 两种形式经 ext_hook 都能还原 ObjectId，服务端也须同样兼容。
		switch n {
		case 12:
			return hex.EncodeToString(raw), nil
		case 24:
			decoded := make([]byte, 12)
			if _, err := hex.Decode(decoded, raw); err != nil {
				return nil, errMsgpack
			}
			return string(raw), nil
		default:
			return nil, errMsgpack
		}
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	case 0xc4, 0xc5, 0xc6:
		// py2 packb(use_bin_type=True) 把 str 编成 bin；键与字符串值一律还原为字符串
		// （实机 quick_login 参数键为 bin，解成 []byte 会令业务层字段全空，2026-10-05）。
		n, err := c.lenN(b)
		if err != nil {
			return nil, err
		}
		if c.i+n > len(c.d) {
			return nil, errMsgpack
		}
		out := string(c.d[c.i : c.i+n])
		c.i += n
		return out, nil
	case 0xca:
		v := math.Float32frombits(binary.BigEndian.Uint32(c.take(4)))
		return float64(v), nil
	case 0xcb:
		v := math.Float64frombits(binary.BigEndian.Uint64(c.take(8)))
		return v, nil
	case 0xcc, 0xcd, 0xce, 0xcf:
		n := 1 << (b - 0xcc)
		raw := c.take(n)
		var v uint64
		for _, x := range raw {
			v = v<<8 | uint64(x)
		}
		return int64(v), nil
	case 0xd0:
		return int64(int8(c.take(1)[0])), nil
	case 0xd1:
		return int64(int16(binary.BigEndian.Uint16(c.take(2)))), nil
	case 0xd2:
		return int64(int32(binary.BigEndian.Uint32(c.take(4)))), nil
	case 0xd3:
		return int64(binary.BigEndian.Uint64(c.take(8))), nil
	case 0xd9, 0xda, 0xdb:
		n, err := c.lenN(b)
		if err != nil {
			return nil, err
		}
		return c.strN(n)
	case 0xdc:
		return c.arrayN(int(binary.BigEndian.Uint16(c.take(2))))
	case 0xdd:
		return c.arrayN(int(binary.BigEndian.Uint32(c.take(4))))
	case 0xde:
		return c.mapN(int(binary.BigEndian.Uint16(c.take(2))))
	case 0xdf:
		return c.mapN(int(binary.BigEndian.Uint32(c.take(4))))
	}
	return nil, errMsgpack
}

func (c *msgpackCursor) lenN(b byte) (int, error) {
	switch b {
	case 0xc4, 0xd9:
		return int(c.take(1)[0]), nil
	case 0xc5, 0xda:
		return int(binary.BigEndian.Uint16(c.take(2))), nil
	case 0xc6, 0xdb:
		return int(binary.BigEndian.Uint32(c.take(4))), nil
	}
	return 0, errMsgpack
}

func (c *msgpackCursor) take(n int) []byte {
	if c.i+n > len(c.d) {
		panic(errMsgpack)
	}
	out := c.d[c.i : c.i+n]
	c.i += n
	return out
}

func (c *msgpackCursor) strN(n int) (string, error) {
	if c.i+n > len(c.d) {
		return "", errMsgpack
	}
	s := string(c.d[c.i : c.i+n])
	c.i += n
	return s, nil
}

func (c *msgpackCursor) arrayN(n int) (any, error) {
	if n < 0 || n > len(c.d)-c.i {
		return nil, errMsgpack
	}
	out := make([]any, 0, n)
	for j := 0; j < n; j++ {
		v, err := c.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (c *msgpackCursor) mapN(n int) (any, error) {
	if n < 0 || n > (len(c.d)-c.i)/2 {
		return nil, errMsgpack
	}
	out := make(map[string]any, n)
	for j := 0; j < n; j++ {
		k, err := c.value()
		if err != nil {
			return nil, err
		}
		v, err := c.value()
		if err != nil {
			return nil, err
		}
		ks, ok := k.(string)
		if !ok {
			ks = fmt.Sprint(k)
		}
		out[ks] = v
	}
	return out, nil
}
