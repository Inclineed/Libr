package ledger

import "time"

// ---------------------------------------------------------------------------
// Relay
// ---------------------------------------------------------------------------

// RelayInfo is a relay entry stored in and returned by the ledger.
type RelayInfo struct {
	PeerID      string    `json:"peer_id"      bson:"peer_id"`
	WSAddress   string    `json:"ws_address"   bson:"ws_address"`
	PublicKey   string    `json:"public_key"   bson:"public_key"`
	LastUpdated time.Time `json:"last_updated" bson:"last_updated"`
}

// upsertRelayRequest is the JSON body accepted by PUT /relays.
type upsertRelayRequest struct {
	PeerID    string `json:"peer_id"`
	WSAddress string `json:"ws_address"`
	Signature string `json:"signature,omitempty"`
}

// ---------------------------------------------------------------------------
// Bootstrap node
// ---------------------------------------------------------------------------

// NodeInfo represents a bootstrap DB node entry.
type NodeInfo struct {
	NodeID    string    `json:"node_id"    bson:"node_id"`
	PeerID    string    `json:"peer_id"    bson:"peer_id"`
	PublicKey string    `json:"public_key" bson:"public_key"`
	LastSeen  time.Time `json:"last_seen"  bson:"last_seen"`
}

// ---------------------------------------------------------------------------
// Mod (moderator)
// ---------------------------------------------------------------------------

// ModInfo represents a currently-online moderator entry.
type ModInfo struct {
	PeerID    string    `json:"peer_id"    bson:"peer_id"`
	PublicKey string    `json:"public_key" bson:"public_key"`
	LastSeen  time.Time `json:"last_seen"  bson:"last_seen"`
}

// ---------------------------------------------------------------------------
// Shared request types
// ---------------------------------------------------------------------------

// authFields are the challenge-response fields required on every write request.
type authFields struct {
	PublicKey string `json:"public_key"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

type nodeRegisterRequest struct {
	authFields
	NodeID string `json:"node_id"`
	PeerID string `json:"peer_id"`
}

type modRegisterRequest struct {
	authFields
	PeerID string `json:"peer_id"`
}

// touchRequest is used by refresh and deregister (only auth fields needed).
type touchRequest struct {
	authFields
}

// ---------------------------------------------------------------------------
// Server config
// ---------------------------------------------------------------------------

// ServerConfig holds optional server-level features.
type ServerConfig struct {
	// RateLimiter, when non-nil, is applied to all incoming requests.
	RateLimiter *RateLimiter
	// ValidateSignatures enables Ed25519 signature verification on PUT /relays.
	ValidateSignatures bool
	// EntryTTL is the window used when filtering live mods and nodes by lastSeen.
	// Zero means return all entries regardless of age.
	EntryTTL time.Duration
}
