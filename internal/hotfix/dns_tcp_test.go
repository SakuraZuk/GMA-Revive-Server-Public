package hotfix

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestServeTCPDNSReply(t *testing.T) {
	query := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x03, 'h', '6', '2', 0x06, 'u', 'p', 'd', 'a', 't', 'e', 0x07, 'n', 'e', 't', 'e', 'a', 's', 'e', 0x03, 'c', 'o', 'm', 0x00, 0x00, 0x01, 0x00, 0x01}
	reply, err := DNSReplyForDomains(query, map[string]net.IP{"netease.com": net.IPv4(192, 168, 31, 50)})
	if err != nil {
		t.Fatalf("DNSReplyForDomains: %v", err)
	}
	if binary.BigEndian.Uint16(reply[6:8]) != 1 {
		t.Fatalf("ancount=%d body=%x", binary.BigEndian.Uint16(reply[6:8]), reply)
	}
}
