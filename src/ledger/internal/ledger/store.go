package ledger

import (
	"context"
	"log"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Store interfaces
// ---------------------------------------------------------------------------

// RelayStore is the persistence interface for relay entries.
type RelayStore interface {
	Upsert(relay RelayInfo) error
	GetAll() ([]RelayInfo, error)
}

// NodeStore is the persistence interface for bootstrap DB node entries.
type NodeStore interface {
	UpsertNode(node NodeInfo) error
	TouchNode(publicKey string) (bool, error) // returns false when not found
	RemoveNode(publicKey string) error
	GetNodes(ttl time.Duration) ([]NodeInfo, error) // ttl=0 → all
}

// ModStore is the persistence interface for online moderator entries.
type ModStore interface {
	IsModAllowed(publicKey string) (bool, error)
	UpsertMod(mod ModInfo) error
	TouchMod(publicKey string) (bool, error) // returns false when not found
	RemoveMod(publicKey string) error
	GetMods(ttl time.Duration) ([]ModInfo, error) // ttl=0 → all
}

// Staler can sweep entries that are older than a cutoff timestamp.
type Staler interface {
	RemoveStaleNodes(cutoff time.Time) (int, error)
	RemoveStaleMods(cutoff time.Time) (int, error)
}

// ---------------------------------------------------------------------------
// InMemoryStore — implements RelayStore, NodeStore, ModStore, Staler
// ---------------------------------------------------------------------------

// InMemoryStore holds all ledger entries in memory with optional TTL eviction.
type InMemoryStore struct {
	// relays
	relaySmu sync.RWMutex
	relays   map[string]RelayInfo

	// bootstrap nodes
	nodeMu sync.RWMutex
	nodes  map[string]NodeInfo // key = publicKey

	// mod allowlist (static — populated at construction or left empty)
	allowMu  sync.RWMutex
	modAllow map[string]struct{}

	// online mods
	modMu sync.RWMutex
	mods  map[string]ModInfo // key = publicKey

	ttl time.Duration // zero means no TTL
}

// NewInMemoryStore creates an InMemoryStore without TTL eviction.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
		nodes:  make(map[string]NodeInfo),
		mods:   make(map[string]ModInfo),
	}
}

// NewInMemoryStoreWithTTL creates an InMemoryStore that evicts entries older
// than ttl.  Call StartCleanup to activate the background goroutine.
func NewInMemoryStoreWithTTL(ttl time.Duration) *InMemoryStore {
	return &InMemoryStore{
		relays: make(map[string]RelayInfo),
		nodes:  make(map[string]NodeInfo),
		mods:   make(map[string]ModInfo),
		ttl:    ttl,
	}
}

// AllowMods pre-populates the in-memory mod allowlist.
func (s *InMemoryStore) AllowMods(publicKeys ...string) {
	s.allowMu.Lock()
	defer s.allowMu.Unlock()
	if s.modAllow == nil {
		s.modAllow = make(map[string]struct{}, len(publicKeys))
	}
	for _, k := range publicKeys {
		s.modAllow[k] = struct{}{}
	}
}

// StartCleanup starts a background goroutine that removes stale entries every
// interval.  It stops when ctx is cancelled.
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

func (s *InMemoryStore) evictStale() {
	cutoff := time.Now().UTC().Add(-s.ttl)

	staleN, _ := s.RemoveStaleNodes(cutoff)
	staleM, _ := s.RemoveStaleMods(cutoff)
	if staleN+staleM > 0 {
		log.Printf("evicted stale entries nodes=%d mods=%d", staleN, staleM)
	}

	s.relaySmu.Lock()
	defer s.relaySmu.Unlock()
	for id, relay := range s.relays {
		if relay.LastUpdated.Before(cutoff) {
			log.Printf("relay evicted (stale) peer_id=%s", id)
			delete(s.relays, id)
		}
	}
}

// ---------------------------------------------------------------------------
// RelayStore impl
// ---------------------------------------------------------------------------

func (s *InMemoryStore) Upsert(relay RelayInfo) error {
	s.relaySmu.Lock()
	defer s.relaySmu.Unlock()

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
	s.relaySmu.RLock()
	defer s.relaySmu.RUnlock()

	relays := make([]RelayInfo, 0, len(s.relays))
	for _, relay := range s.relays {
		relays = append(relays, relay)
	}
	return relays, nil
}

// ---------------------------------------------------------------------------
// NodeStore impl
// ---------------------------------------------------------------------------

func (s *InMemoryStore) UpsertNode(node NodeInfo) error {
	s.nodeMu.Lock()
	defer s.nodeMu.Unlock()
	node.LastSeen = time.Now().UTC()
	s.nodes[node.PublicKey] = node
	log.Printf("node upserted peer_id=%s node_id=%s", node.PeerID, node.NodeID)
	return nil
}

func (s *InMemoryStore) TouchNode(publicKey string) (bool, error) {
	s.nodeMu.Lock()
	defer s.nodeMu.Unlock()
	n, ok := s.nodes[publicKey]
	if !ok {
		return false, nil
	}
	n.LastSeen = time.Now().UTC()
	s.nodes[publicKey] = n
	return true, nil
}

func (s *InMemoryStore) RemoveNode(publicKey string) error {
	s.nodeMu.Lock()
	defer s.nodeMu.Unlock()
	delete(s.nodes, publicKey)
	return nil
}

func (s *InMemoryStore) GetNodes(ttl time.Duration) ([]NodeInfo, error) {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	cutoff := time.Now().UTC().Add(-ttl)
	nodes := make([]NodeInfo, 0, len(s.nodes))
	for _, n := range s.nodes {
		if ttl == 0 || n.LastSeen.After(cutoff) {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// ---------------------------------------------------------------------------
// ModStore impl
// ---------------------------------------------------------------------------

func (s *InMemoryStore) IsModAllowed(publicKey string) (bool, error) {
	s.allowMu.RLock()
	defer s.allowMu.RUnlock()
	if s.modAllow == nil {
		return false, nil
	}
	_, ok := s.modAllow[publicKey]
	return ok, nil
}

func (s *InMemoryStore) UpsertMod(mod ModInfo) error {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	mod.LastSeen = time.Now().UTC()
	s.mods[mod.PublicKey] = mod
	log.Printf("mod upserted peer_id=%s public_key=%s", mod.PeerID, mod.PublicKey)
	return nil
}

func (s *InMemoryStore) TouchMod(publicKey string) (bool, error) {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	m, ok := s.mods[publicKey]
	if !ok {
		return false, nil
	}
	m.LastSeen = time.Now().UTC()
	s.mods[publicKey] = m
	return true, nil
}

func (s *InMemoryStore) RemoveMod(publicKey string) error {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	delete(s.mods, publicKey)
	return nil
}

func (s *InMemoryStore) GetMods(ttl time.Duration) ([]ModInfo, error) {
	s.modMu.RLock()
	defer s.modMu.RUnlock()
	cutoff := time.Now().UTC().Add(-ttl)
	mods := make([]ModInfo, 0, len(s.mods))
	for _, m := range s.mods {
		if ttl == 0 || m.LastSeen.After(cutoff) {
			mods = append(mods, m)
		}
	}
	return mods, nil
}

// ---------------------------------------------------------------------------
// Staler impl
// ---------------------------------------------------------------------------

func (s *InMemoryStore) RemoveStaleNodes(cutoff time.Time) (int, error) {
	s.nodeMu.Lock()
	defer s.nodeMu.Unlock()
	var n int
	for k, node := range s.nodes {
		if node.LastSeen.Before(cutoff) {
			delete(s.nodes, k)
			n++
		}
	}
	return n, nil
}

func (s *InMemoryStore) RemoveStaleMods(cutoff time.Time) (int, error) {
	s.modMu.Lock()
	defer s.modMu.Unlock()
	var n int
	for k, mod := range s.mods {
		if mod.LastSeen.Before(cutoff) {
			delete(s.mods, k)
			n++
		}
	}
	return n, nil
}
