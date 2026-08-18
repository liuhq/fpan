package auth

import (
	"sync"
	"time"
)

const sessionLifetime = 7 * 24 * time.Hour

type Sessions struct {
	mu       sync.RWMutex
	sessions map[string]time.Time
}

func NewSessions() *Sessions {
	return &Sessions{
		sessions: make(map[string]time.Time),
	}
}

func (s *Sessions) Create() (string, error) {
	id, err := randomString()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[id] = time.Now().Add(sessionLifetime)

	return id, nil
}

func (s *Sessions) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, id)
}

func (s *Sessions) Valid(id string) bool {
	s.mu.RLock()
	expiresAt, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	if time.Now().After(expiresAt) {
		s.Delete(id)
		return false
	}

	return true
}
