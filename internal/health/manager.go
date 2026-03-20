package health

import (
	"sync"
	"time"
)

type Manager struct {
	mu           sync.RWMutex
	ejectedUntil map[string]time.Time
}

func NewManager() *Manager {
	return &Manager{
		ejectedUntil: make(map[string]time.Time),
	}
}

func (m *Manager) Eject(upstream string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ejectedUntil[upstream] = time.Now().Add(duration)
}

func (m *Manager) IsEligible(upstream string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	until, ok := m.ejectedUntil[upstream]
	if !ok {
		return true
	}
	if time.Now().After(until) {
		delete(m.ejectedUntil, upstream)
		return true
	}

	return false
}
