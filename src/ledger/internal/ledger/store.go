package ledger

import (
	"log"
	"sync"
	"time"
)

type RelayStore interface {
	Upsert(relay RelayInfo) error
	GetAll() ([]RelayInfo, error)
}

type InMemoryStore struct {
	mu     sync.RWMutex
	relays map[string]RelayInfo
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
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

	// TODO: Add persistent storage backend support (e.g., MongoDB) implementing RelayStore.
	// TODO: Add optional relay signature validation during upsert.
	// TODO: Add rate limiting for relay registration endpoints.
	return nil
}

func (s *InMemoryStore) GetAll() ([]RelayInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	relays := make([]RelayInfo, 0, len(s.relays))
	for _, relay := range s.relays {
		relays = append(relays, relay)
	}

	// TODO: Add TTL cleanup to remove stale relays after a configurable duration.
	return relays, nil
}
