package main

import (
	"net"
	"testing"
	"time"

	"hs-server/internal/mobileproto"
	"hs-server/internal/session"
)

// 原固定截止时间会在第三轮前断开；当前连接收到完整帧后续期，停止发帧才回收。
func TestActiveConnectionRenewsIdleDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	svc := testService(t)
	go func() {
		serveWithIdleTimeout(server, session.NewManager(1), svc, nil, 1<<20, 300*time.Millisecond)
		close(done)
	}()
	wire := frame(t, mobileproto.Frame{Command: mobileproto.CmdSeedRequest})
	for i := 0; i < 4; i++ {
		if i > 0 {
			time.Sleep(140 * time.Millisecond)
		}
		_ = client.SetDeadline(time.Now().Add(time.Second))
		if _, err := client.Write(wire); err != nil {
			t.Fatalf("活跃连接第 %d 轮提前关闭：%v", i, err)
		}
		buffer := make([]byte, 100)
		if _, err := client.Read(buffer); err != nil {
			t.Fatalf("第 %d 轮没有响应：%v", i, err)
		}
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("空闲连接没有及时回收")
	}
}
