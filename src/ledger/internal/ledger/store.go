package ledger

import (
	"context"
	"log"
	"sync"
	"time"
)

// RelayStore is the persistence interface for relay entries.
type RelayStore interface {
	Upsert(relay RelayInfo) error
	GetAll() ([]RelayInfo, error)
}

// InMemoryStore holds relay entries in memory with optional TTL eviction.
type InMemoryStore struct {
	mu     sync.RWMutex
	relays map[string]RelayInfo
	ttl    time.Duration // zero means no TTL
}

// NewInMemoryStore creates an InMemoryStore without TTL eviction.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
	}
}

// NewInMemoryStoreWithTTL creates an InMemoryStore that evicts entries
// older than ttl.  Call StartCleanup to activate background eviction.
func NewInMemoryStoreWithTTL(ttl time.Duration) *InMemoryStore {
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
		ttl:    ttl,
	}
}

// StartCleanup starts a background goroutine that removes stale relays every
// interval.  It stops when ctx is cancelled.  Call this once per store.
func (s *InMemoryStore) StartCleanup(ctx context.Context, interval time.Duration) {
	if s.ttl <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.evictStale()
			}
		}
	}()
}

// evictStale removes all relays whose LastUpdated is older than s.ttl.
func (s *InMemoryStore) evictStale() {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, relay := range s.relays {
		if now.Sub(relay.LastUpdated) > s.ttl {
			log.Printf("relay evicted (stale) peer_id=%s", id)
			delete(s.relays, id)
		}
	}
}

func (s *InMemoryStore) Upsert(relay RelayInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.relays[relay.PeerID]
	relay.LastUpdated = time.Now().UTC()
	s.relays[relay.PeerID] = relay

	if exists {
		if existing.WSAddress != relay.WSAddress {
			log.Printf("relay updated peer_id=%s ws_address=%s", relay.PeerID, relay.WSAddress)
		} else {
			log.Printf("relay refreshed peer_id=%s", relay.PeerID)
		}
	} else {
		log.Printf("relay registered peer_id=%s ws_address=%s", relay.PeerID, relay.WSAddress)
	}

	return nil
}

func (s *InMemoryStore) GetAll() ([]RelayInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	relays := make([]RelayInfo, 0, len(s.relays))
	for _, relay := range s.relays {
		relays = append(relays, relay)
	}
	return relays, nil
}
