package hotfix

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

// DNSReply 仅解析单问题非压缩查询。非项目域名返回 REFUSED，不做开放递归。
func DNSReply(query []byte, answer net.IP) ([]byte, error) {
	return DNSReplyForDomains(query, map[string]net.IP{
		"netease.com": answer,
		"easebar.com": answer,
	})
}

// DNSReplyForDomains 根据最长匹配的域名后缀返回 IPv4。只支持单问题 A/AAAA
// 查询，不递归，不接受压缩问题名；未配置后缀的域名返回 REFUSED。
func DNSReplyForDomains(query []byte, answers map[string]net.IP) ([]byte, error) {
	if len(query) < 12 || binary.BigEndian.Uint16(query[4:6]) != 1 || query[2]&0xf8 != 0 {
		return nil, errors.New("不支持的 DNS 查询")
	}
	i := 12
	var labels []string
	for {
		if i >= len(query) {
			return nil, errors.New("DNS 名称截断")
		}
		n := int(query[i])
		i++
		if n == 0 {
			break
		}
		if n > 63 || i+n > len(query) || i+n > 267 {
			return nil, errors.New("DNS 标签不合法")
		}
		labels = append(labels, strings.ToLower(string(query[i:i+n])))
		i += n
	}
	if i+4 > len(query) {
		return nil, errors.New("DNS 类型或查询类别不完整")
	}
	typeID, class := binary.BigEndian.Uint16(query[i:i+2]), binary.BigEndian.Uint16(query[i+2:i+4])
	name := strings.Join(labels, ".")
	var answer net.IP
	longest := ""
	for suffix, candidate := range answers {
		suffix = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(suffix)), ".")
		if suffix != "" && (name == suffix || strings.HasSuffix(name, "."+suffix)) && len(suffix) > len(longest) {
			longest, answer = suffix, candidate
		}
	}
	reply := append([]byte(nil), query[:i+4]...)
	reply[2], reply[3] = 0x80|(query[2]&1), 0
	for j := 6; j < 12; j++ {
		reply[j] = 0
	}
	if longest == "" {
		reply[3] = 5
		return reply, nil
	}
	reply[2] |= 4 // 权威回答。
	if typeID != 1 || class != 1 || answer.To4() == nil {
		return reply, nil
	}
	binary.BigEndian.PutUint16(reply[6:8], 1)
	reply = append(reply, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
	return append(reply, answer.To4()...), nil
}

func ServeDNS(conn *net.UDPConn, answer net.IP) error {
	return ServeDNSWithAnswers(conn, map[string]net.IP{
		"netease.com": answer,
		"easebar.com": answer,
	})
}

func ServeDNSWithAnswers(conn *net.UDPConn, answers map[string]net.IP) error {
	buf := make([]byte, 4096)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		reply, err := DNSReplyForDomains(buf[:n], answers)
		if err != nil {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.WriteToUDP(reply, remote)
	}
}

// ServeTCPDNS 提供 RFC 7766 TCP DNS（2 字节长度前缀帧），供 UDP 出站被劫持的
// 联调环境走 TCP 回退。
func ServeTCPDNS(ln net.Listener, answers map[string]net.IP) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			var lenBuf [2]byte
			for {
				if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
					return
				}
				n := int(binary.BigEndian.Uint16(lenBuf[:]))
				if n == 0 || n > 4096 {
					return
				}
				query := make([]byte, n)
				if _, err := io.ReadFull(c, query); err != nil {
					return
				}
				reply, err := DNSReplyForDomains(query, answers)
				if err != nil {
					return
				}
				out := make([]byte, 2+len(reply))
				binary.BigEndian.PutUint16(out[:2], uint16(len(reply)))
				copy(out[2:], reply)
				if _, err := c.Write(out); err != nil {
					return
				}
			}
		}(conn)
	}
}
