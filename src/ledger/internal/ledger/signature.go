package ledger

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
)

// SignatureValidator verifies that relay upsert requests are signed by the
// private key corresponding to the peer's declared public key (peer_id).
//
// Convention:
//
//	peer_id  = base64.StdEncoding of the 32-byte Ed25519 public key
//	signature = base64.StdEncoding of the 64-byte Ed25519 signature over
//	            the message: "<peer_id>:<ws_address>"
type SignatureValidator struct{}

// Validate returns nil when the signature is authentic, or an error
// describing why validation failed.
func (v *SignatureValidator) Validate(req upsertRelayRequest) error {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(req.PeerID)
	if err != nil {
		return fmt.Errorf("peer_id is not valid base64: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("peer_id decoded to %d bytes, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	if req.Signature == "" {
		return errors.New("signature is required when signature validation is enabled")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return fmt.Errorf("signature is not valid base64: %w", err)
	}

	message := req.PeerID + ":" + req.WSAddress
	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), []byte(message), sigBytes) {
		return errors.New("signature verification failed")
	}

	return nil
}
