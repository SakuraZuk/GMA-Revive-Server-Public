package mobileproto

import "encoding/binary"

const (
	WireVarint = 0
	WireBytes  = 2
)

type Field struct {
	Number uint32
	Wire   uint8
	Varint uint64
	Bytes  []byte
}

func EncodeVarint(v uint64) []byte {
	var b [10]byte
	n := binary.PutUvarint(b[:], v)
	return b[:n]
}

func AppendField(dst []byte, number uint32, wire uint8, value []byte) []byte {
	dst = append(dst, EncodeVarint(uint64(number<<3)|uint64(wire))...)
	if wire == WireBytes {
		dst = append(dst, EncodeVarint(uint64(len(value)))...)
	}
	return append(dst, value...)
}

func AppendVarintField(dst []byte, number uint32, value uint64) []byte {
	return AppendField(dst, number, WireVarint, EncodeVarint(value))
}

// Fields decodes only protobuf wire types used by the verified common.proto.
// Unknown fields are preserved as bytes/varints for forward compatibility.
func Fields(data []byte) ([]Field, error) {
	var out []Field
	for i := 0; i < len(data); {
		tag, n := binary.Uvarint(data[i:])
		if n <= 0 {
			return nil, ErrInvalidFrame
		}
		i += n
		field := Field{Number: uint32(tag >> 3), Wire: uint8(tag & 7)}
		switch field.Wire {
		case WireVarint:
			v, m := binary.Uvarint(data[i:])
			if m <= 0 {
				return nil, ErrInvalidFrame
			}
			field.Varint = v
			i += m
		case WireBytes:
			l, m := binary.Uvarint(data[i:])
			if m <= 0 || l > uint64(len(data)-i-m) {
				return nil, ErrInvalidFrame
			}
			i += m
			field.Bytes = append([]byte(nil), data[i:i+int(l)]...)
			i += int(l)
		default:
			return nil, ErrInvalidFrame
		}
		out = append(out, field)
	}
	return out, nil
}

func First(fields []Field, number uint32) (Field, bool) {
	for _, f := range fields {
		if f.Number == number {
			return f, true
		}
	}
	return Field{}, false
}
