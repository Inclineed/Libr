package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// NonceStore issues and consumes one-time challenge nonces used in the
// Ed25519 challenge-response auth flow.
type NonceStore interface {
	// Issue generates a fresh nonce for publicKey, replacing any previous one.
	Issue(publicKey string) (string, error)
	// Consume returns the stored nonce and removes it (one-time use).
	// Returns ("", false) if no valid nonce exists or it has expired.
	Consume(publicKey string) (string, bool)
}

type nonceEntry struct {
	value  string
	expiry time.Time
}

// InMemoryNonceStore stores nonces in memory with a configurable TTL.
type InMemoryNonceStore struct {
	mu      sync.Mutex
	entries map[string]nonceEntry
	ttl     time.Duration
}

// NewInMemoryNonceStore creates a nonce store where each nonce expires after ttl.
func NewInMemoryNonceStore(ttl time.Duration) *InMemoryNonceStore {
	return &InMemoryNonceStore{
		entries: make(map[string]nonceEntry),
		ttl:     ttl,
	}
}

func (s *InMemoryNonceStore) Issue(publicKey string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(b)

	s.mu.Lock()
	s.entries[publicKey] = nonceEntry{value: nonce, expiry: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return nonce, nil
}

func (s *InMemoryNonceStore) Consume(publicKey string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[publicKey]
	if !ok {
		return "", false
	}
	delete(s.entries, publicKey)
	if time.Now().After(entry.expiry) {
		return "", false
	}
	return entry.value, true
}
