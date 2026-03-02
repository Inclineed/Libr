package ledger

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// LedgerStores groups all persistence backends used by the server.
type LedgerStores struct {
	Relays RelayStore
	Nodes  NodeStore
	Mods   ModStore
	Nonces NonceStore
}

// Server is the HTTP server for the relay ledger.
type Server struct {
	stores LedgerStores
	mux    *http.ServeMux
	cfg    ServerConfig
	sigVal SignatureValidator
}

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	Status string `json:"status"`
}

// NewServer creates a Server backed by a RelayStore; nodes/mods use the same
// store if it also implements NodeStore and ModStore (e.g. *InMemoryStore).
func NewServer(store RelayStore) *Server {
	stores := LedgerStores{Relays: store}
	if ns, ok := store.(NodeStore); ok {
		stores.Nodes = ns
	}
	if ms, ok := store.(ModStore); ok {
		stores.Mods = ms
	}
	return NewServerWithConfig(stores, ServerConfig{})
}

// NewServerWithConfig creates a Server with the provided stores and config.
// A default in-memory nonce store (60 s TTL) is used if Nonces is nil.
func NewServerWithConfig(stores LedgerStores, cfg ServerConfig) *Server {
	if stores.Nonces == nil {
		stores.Nonces = NewInMemoryNonceStore(60 * 1e9)
	}
	s := &Server{
		stores: stores,
		mux:    http.NewServeMux(),
		cfg:    cfg,
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RateLimiter != nil {
		s.cfg.RateLimiter.Middleware(s.mux).ServeHTTP(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)

	// Relays (existing — used by Go relay server)
	s.mux.HandleFunc("/relays", s.handleRelays)

	// Auth challenge
	s.mux.HandleFunc("/auth/challenge", s.handleChallenge)

	// Mods read
	s.mux.HandleFunc("/mods", s.handleGetMods)
	s.mux.HandleFunc("/mods/check", s.handleCheckMod)
	// Mods write
	s.mux.HandleFunc("/mods/register", s.handleModRegister)
	s.mux.HandleFunc("/mods/refresh", s.handleModRefresh)
	s.mux.HandleFunc("/mods/deregister", s.handleModDeregister)

	// Nodes read
	s.mux.HandleFunc("/nodes", s.handleGetNodes)
	// Nodes write
	s.mux.HandleFunc("/nodes/register", s.handleNodeRegister)
	s.mux.HandleFunc("/nodes/refresh", s.handleNodeRefresh)
	s.mux.HandleFunc("/nodes/deregister", s.handleNodeDeregister)
}

// ---------------------------------------------------------------------------
// /health
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// ---------------------------------------------------------------------------
// /auth/challenge
// ---------------------------------------------------------------------------

// GET /auth/challenge?public_key=<base64>
func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pk := r.URL.Query().Get("public_key")
	if pk == "" {
		writeError(w, http.StatusBadRequest, "public_key query parameter is required")
		return
	}
	nonce, err := s.stores.Nonces.Issue(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue challenge")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce})
}

// verifyAuth validates the Ed25519 challenge-response fields.
// If valid it returns the verified public key; otherwise it writes an error
// response and returns "".
func (s *Server) verifyAuth(w http.ResponseWriter, af authFields) string {
	if af.PublicKey == "" || af.Nonce == "" || af.Signature == "" {
		writeError(w, http.StatusBadRequest, "public_key, nonce and signature are required")
		return ""
	}
	stored, ok := s.stores.Nonces.Consume(af.PublicKey)
	if !ok {
		writeError(w, http.StatusUnauthorized, "no valid challenge found — request a new challenge first")
		return ""
	}
	if stored != af.Nonce {
		writeError(w, http.StatusUnauthorized, "nonce mismatch")
		return ""
	}
	pubKeyBytes, err := base64.StdEncoding.DecodeString(af.PublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		writeError(w, http.StatusBadRequest, "public_key is not a valid base64 Ed25519 key")
		return ""
	}
	sigBytes, err := base64.StdEncoding.DecodeString(af.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signature is not valid base64")
		return ""
	}
	if !ed25519.Verify(pubKeyBytes, []byte(af.Nonce), sigBytes) {
		writeError(w, http.StatusUnauthorized, "invalid Ed25519 signature")
		return ""
	}
	return af.PublicKey
}

// ---------------------------------------------------------------------------
// /relays
// ---------------------------------------------------------------------------

func (s *Server) handleRelays(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.handleUpsertRelay(w, r)
	case http.MethodGet:
		s.handleGetRelays(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUpsertRelay(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req upsertRelayRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if req.PeerID == "" {
		writeError(w, http.StatusBadRequest, "peer_id must not be empty")
		return
	}
	if req.WSAddress == "" {
		writeError(w, http.StatusBadRequest, "ws_address must not be empty")
		return
	}
	if s.cfg.ValidateSignatures {
		if err := s.sigVal.Validate(req); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid signature: "+err.Error())
			return
		}
	}
	if err := s.stores.Relays.Upsert(RelayInfo{PeerID: req.PeerID, WSAddress: req.WSAddress}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upsert relay")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

func (s *Server) handleGetRelays(w http.ResponseWriter, _ *http.Request) {
	relays, err := s.stores.Relays.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch relays")
		return
	}
	writeJSON(w, http.StatusOK, relays)
}

// ---------------------------------------------------------------------------
// /mods  (read)
// ---------------------------------------------------------------------------

// GET /mods
func (s *Server) handleGetMods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.stores.Mods == nil {
		writeJSON(w, http.StatusOK, []ModInfo{})
		return
	}
	mods, err := s.stores.Mods.GetMods(s.cfg.EntryTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch mods")
		return
	}
	writeJSON(w, http.StatusOK, mods)
}

// GET /mods/check?public_key=<b64>
func (s *Server) handleCheckMod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pk := r.URL.Query().Get("public_key")
	if pk == "" {
		writeError(w, http.StatusBadRequest, "public_key query parameter is required")
		return
	}
	if s.stores.Mods == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"allowed": false})
		return
	}
	allowed, err := s.stores.Mods.IsModAllowed(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check mod allowlist")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": allowed})
}

// ---------------------------------------------------------------------------
// /mods  (write)
// ---------------------------------------------------------------------------

// POST /mods/register
func (s *Server) handleModRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req modRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if req.PeerID == "" {
		writeError(w, http.StatusBadRequest, "peer_id is required")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	allowed, err := s.stores.Mods.IsModAllowed(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check mod allowlist")
		return
	}
	if !allowed {
		log.Printf("mod registration denied: not in allowlist public_key=%s", pk)
		writeError(w, http.StatusForbidden, "public key is not in the moderator allowlist")
		return
	}
	if err := s.stores.Mods.UpsertMod(ModInfo{PeerID: req.PeerID, PublicKey: pk}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register mod")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// POST /mods/refresh
func (s *Server) handleModRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req touchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	allowed, err := s.stores.Mods.IsModAllowed(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check mod allowlist")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "public key is not in the moderator allowlist")
		return
	}
	found, err := s.stores.Mods.TouchMod(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh mod")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "mod entry not found — register first")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// POST /mods/deregister
func (s *Server) handleModDeregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req touchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	allowed, err := s.stores.Mods.IsModAllowed(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check mod allowlist")
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "public key is not in the moderator allowlist")
		return
	}
	if err := s.stores.Mods.RemoveMod(pk); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to deregister mod")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// ---------------------------------------------------------------------------
// /nodes  (read)
// ---------------------------------------------------------------------------

// GET /nodes
func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.stores.Nodes == nil {
		writeJSON(w, http.StatusOK, []NodeInfo{})
		return
	}
	nodes, err := s.stores.Nodes.GetNodes(s.cfg.EntryTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// ---------------------------------------------------------------------------
// /nodes  (write)
// ---------------------------------------------------------------------------

// POST /nodes/register
func (s *Server) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req nodeRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if req.NodeID == "" || req.PeerID == "" {
		writeError(w, http.StatusBadRequest, "node_id and peer_id are required")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	if err := s.stores.Nodes.UpsertNode(NodeInfo{NodeID: req.NodeID, PeerID: req.PeerID, PublicKey: pk}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register node")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// POST /nodes/refresh
func (s *Server) handleNodeRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req touchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	found, err := s.stores.Nodes.TouchNode(pk)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to refresh node")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "node entry not found — register first")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// POST /nodes/deregister
func (s *Server) handleNodeDeregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req touchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	pk := s.verifyAuth(w, req.authFields)
	if pk == "" {
		return
	}
	if err := s.stores.Nodes.RemoveNode(pk); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to deregister node")
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
