package mobileproto

import "testing"

func TestFrameRoundTripAndPartialRead(t *testing.T) {
	want := Frame{Command: 2, Payload: []byte{1, 2, 3}}
	wire, err := Encode(want, 16)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, complete, err := Decode(wire[:4], 16); err != nil || complete {
		t.Fatalf("partial frame: complete=%v err=%v", complete, err)
	}
	got, used, complete, err := Decode(wire, 16)
	if err != nil || !complete || used != len(wire) {
		t.Fatalf("decode: used=%d complete=%v err=%v", used, complete, err)
	}
	if got.Command != want.Command || string(got.Payload) != string(want.Payload) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestFrameRejectsOversize(t *testing.T) {
	if _, err := Encode(Frame{Payload: make([]byte, 17)}, 16); err != ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}
