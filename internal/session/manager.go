package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrCapacity = errors.New("session capacity reached")

type Session struct {
	ID       string
	Created  time.Time
	Touched  time.Time
	ClientIP string
}

type Manager struct {
	mu       sync.RWMutex
	max      int
	sessions map[string]Session
}

func NewManager(max int) *Manager {
	if max < 1 {
		max = 1
	}
	return &Manager{max: max, sessions: make(map[string]Session, max)}
}

func (m *Manager) New(clientIP string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) >= m.max {
		return Session{}, ErrCapacity
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	s := Session{ID: hex.EncodeToString(raw[:]), Created: now, Touched: now, ClientIP: clientIP}
	m.sessions[s.ID] = s
	return s, nil
}

func (m *Manager) Touch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return false
	}
	s.Touched = time.Now().UTC()
	m.sessions[id] = s
	return true
}

func (m *Manager) Remove(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return false
	}
	delete(m.sessions, id)
	return true
}

func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// ExpireIdle 清理管理接口遗留的会话；TCP 会话仍在连接结束时主动删除。
func (m *Manager) ExpireIdle(before time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, s := range m.sessions {
		if s.Touched.Before(before) {
			delete(m.sessions, id)
			removed++
		}
	}
	return removed
}
