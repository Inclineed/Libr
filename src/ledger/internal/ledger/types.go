package ledger

import "time"

// RelayInfo is a relay entry stored in and returned by the ledger.
type RelayInfo struct {
	PeerID      string    `json:"peer_id"`
	WSAddress   string    `json:"ws_address"`
	LastUpdated time.Time `json:"last_updated"`
}

// upsertRelayRequest is the JSON body accepted by PUT /relays.
// Signature is optional unless the server is configured with
// ValidateSignatures = true, in which case it must be a base64-encoded
// Ed25519 signature over "<peer_id>:<ws_address>".
type upsertRelayRequest struct {
	PeerID    string `json:"peer_id"`
	WSAddress string `json:"ws_address"`
	Signature string `json:"signature,omitempty"`
}

// ServerConfig holds optional server-level features.
type ServerConfig struct {
	// RateLimiter, when non-nil, is applied to all incoming requests.
	RateLimiter *RateLimiter
	// ValidateSignatures enables Ed25519 signature verification on PUT /relays.
	// When true, each request must carry a valid Signature field.
	ValidateSignatures bool
}
