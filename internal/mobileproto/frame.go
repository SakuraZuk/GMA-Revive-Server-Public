// Package mobileproto 提供已验证的 MobileRPC 帧、protobuf wire、BSON 与实体消息编码。
// 协议依据 out/gate-protocol-spec.md；已恢复业务签名见 internal/game。
package mobileproto

import (
	"encoding/binary"
	"errors"
)

const HeaderSize = 6

var (
	ErrFrameTooLarge = errors.New("mobileproto: frame too large")
	ErrInvalidFrame  = errors.New("mobileproto: invalid frame")
)

type Frame struct {
	Command uint16
	Payload []byte
}

func Encode(f Frame, maxPayload int) ([]byte, error) {
	if len(f.Payload) > maxPayload {
		return nil, ErrFrameTooLarge
	}
	total := 2 + len(f.Payload)
	out := make([]byte, 4+total)
	binary.LittleEndian.PutUint32(out[:4], uint32(total))
	binary.LittleEndian.PutUint16(out[4:6], f.Command)
	copy(out[6:], f.Payload)
	return out, nil
}

// Decode consumes one frame from buf. It returns consumed bytes and whether a
// complete frame was present. Partial network reads are expected.
func Decode(buf []byte, maxPayload int) (Frame, int, bool, error) {
	if len(buf) < HeaderSize {
		return Frame{}, 0, false, nil
	}
	total := int(binary.LittleEndian.Uint32(buf[:4]))
	if total < 2 || total-2 > maxPayload {
		return Frame{}, 0, false, ErrInvalidFrame
	}
	needed := 4 + total
	if len(buf) < needed {
		return Frame{}, 0, false, nil
	}
	payload := make([]byte, total-2)
	copy(payload, buf[6:needed])
	return Frame{Command: binary.LittleEndian.Uint16(buf[4:6]), Payload: payload}, needed, true, nil
}
