package session

import (
	"testing"
	"time"
)

func TestManagerCapacityAndLifecycle(t *testing.T) {
	m := NewManager(1)
	s, err := m.New("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.New("127.0.0.1"); err != ErrCapacity {
		t.Fatalf("expected ErrCapacity, got %v", err)
	}
	if !m.Touch(s.ID) || !m.Remove(s.ID) || m.Len() != 0 {
		t.Fatalf("session lifecycle failed")
	}
}

func TestExpireIdleReleasesCapacity(t *testing.T) {
	m := NewManager(1)
	if _, err := m.New("127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if removed := m.ExpireIdle(time.Now().Add(-time.Hour)); removed != 0 {
		t.Fatal("新会话不应过期")
	}
	if removed := m.ExpireIdle(time.Now().Add(time.Hour)); removed != 1 {
		t.Fatal("过期会话未回收")
	}
	if _, err := m.New("127.0.0.1"); err != nil {
		t.Fatal("回收后容量仍被占用")
	}
}
