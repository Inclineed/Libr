package ledger

import (
	"context"
	"log"
	"sync"
	"time"
)

// DefaultRelayTTL is 2× the relay ledger-sync period (30 s), giving each relay
// two full sync cycles before it is considered stale.
const DefaultRelayTTL = 2 * time.Minute

type RelayStore interface {
	Upsert(relay RelayInfo) error
	GetAll() ([]RelayInfo, error)
}

type InMemoryStore struct {
	mu     sync.RWMutex
	relays map[string]RelayInfo
	ttl    time.Duration
}

// NewInMemoryStore creates a store with DefaultRelayTTL.
func NewInMemoryStore() *InMemoryStore {
	return NewInMemoryStoreWithTTL(DefaultRelayTTL)
}

// NewInMemoryStoreWithTTL creates a store where relays that have not been
// refreshed within ttl are considered stale.
func NewInMemoryStoreWithTTL(ttl time.Duration) *InMemoryStore {
	if ttl <= 0 {
		ttl = DefaultRelayTTL
	}
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
		ttl:    ttl,
	}
}

// StartCleanup launches a background goroutine that periodically evicts stale
// relays.  It returns immediately; the goroutine exits when ctx is cancelled.
func (s *InMemoryStore) StartCleanup(ctx context.Context) {
	interval := s.ttl / 2
	if interval < time.Second {
		interval = time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.evictStale()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// evictStale removes relays whose LastUpdated timestamp is older than s.ttl.
func (s *InMemoryStore) evictStale() {
	cutoff := time.Now().UTC().Add(-s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()

	for peerID, relay := range s.relays {
		if relay.LastUpdated.Before(cutoff) {
			delete(s.relays, peerID)
			log.Printf("relay evicted peer_id=%s last_seen=%s", peerID, relay.LastUpdated.Format(time.RFC3339))
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

	cutoff := time.Now().UTC().Add(-s.ttl)
	relays := make([]RelayInfo, 0, len(s.relays))
	for _, relay := range s.relays {
		// Exclude relays that have not been refreshed within the TTL window.
		if !relay.LastUpdated.Before(cutoff) {
			relays = append(relays, relay)
		}
	}
	return relays, nil
}
