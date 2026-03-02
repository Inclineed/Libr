package ledger

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

type Server struct {
	store RelayStore
	mux   *http.ServeMux
}

type errorResponse struct {
	Error string `json:"error"`
}

type successResponse struct {
	Status string `json:"status"`
}

func NewServer(store RelayStore) *Server {
	s := &Server{
		store: store,
		mux:   http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/relays", s.handleRelays)
	s.mux.HandleFunc("/health", s.handleHealth)
}

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
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
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

	relay := RelayInfo{
		PeerID:    req.PeerID,
		WSAddress: req.WSAddress,
	}

	if err := s.store.Upsert(relay); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upsert relay")
		return
	}

	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
}

func (s *Server) handleGetRelays(w http.ResponseWriter, _ *http.Request) {
	relays, err := s.store.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch relays")
		return
	}

	writeJSON(w, http.StatusOK, relays)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, successResponse{Status: "ok"})
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
