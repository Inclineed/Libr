package ledger

import "time"

type RelayInfo struct {
	PeerID      string    `json:"peer_id"`
	WSAddress   string    `json:"ws_address"`
	LastUpdated time.Time `json:"last_updated"`
}

type upsertRelayRequest struct {
	PeerID    string `json:"peer_id"`
	WSAddress string `json:"ws_address"`
}
