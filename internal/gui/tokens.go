package gui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const (
	ticketLifetime  = time.Minute
	sessionLifetime = 12 * time.Hour
	maxTokens       = 1024
)

var errTicket = errors.New("invalid or expired ticket; open the interface again")

type tokens struct {
	mu       sync.Mutex
	now      func() time.Time
	tickets  map[[32]byte]time.Time
	sessions map[[32]byte]time.Time
}

func newTokens() *tokens {
	return &tokens{now: time.Now, tickets: make(map[[32]byte]time.Time), sessions: make(map[[32]byte]time.Time)}
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func (t *tokens) prune() {
	now := t.now()
	for _, entries := range []map[[32]byte]time.Time{t.tickets, t.sessions} {
		for key, deadline := range entries {
			if !now.Before(deadline) {
				delete(entries, key)
			}
		}
	}
}

func (t *tokens) ticket() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.prune()
	if len(t.tickets) >= maxTokens {
		return "", errors.New("too many pending browser logins; try again in one minute")
	}
	value, err := randomToken()
	if err == nil {
		t.tickets[sha256.Sum256([]byte(value))] = t.now().Add(ticketLifetime)
	}
	return value, err
}

func (t *tokens) exchange(ticket string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.prune()
	key := sha256.Sum256([]byte(ticket))
	if _, ok := t.tickets[key]; !ok || len(t.sessions) >= maxTokens {
		return "", errTicket
	}
	value, err := randomToken()
	if err != nil {
		return "", err
	}
	delete(t.tickets, key)
	t.sessions[sha256.Sum256([]byte(value))] = t.now().Add(sessionLifetime)
	return value, nil
}

func (t *tokens) authenticated(value string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	deadline, ok := t.sessions[sha256.Sum256([]byte(value))]
	return ok && t.now().Before(deadline)
}

func (t *tokens) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.tickets)
	clear(t.sessions)
}
