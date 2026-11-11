package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
)

func (s *Server) handleDiscoverySearch(w http.ResponseWriter, r *http.Request) {
	var filter dbinfra.DiscoveryFilter
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&filter); err != nil {
		writeError(w, http.StatusBadRequest, "invalid discovery request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid discovery request")
		return
	}
	if filter.Limit == 0 {
		filter.Limit = 25
	}
	page, err := s.repo.SearchDiscovery(r.Context(), filter)
	if err != nil {
		var ambiguity *dbinfra.DiscoveryRoomAmbiguity
		switch {
		case errors.Is(err, dbinfra.ErrDiscoveryUnavailable):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		case errors.As(err, &ambiguity):
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "candidates": ambiguity.Candidates})
		case errors.Is(err, dbinfra.ErrDiscoveryCursor), errors.Is(err, dbinfra.ErrDiscoveryBindingConflict):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, dbinfra.ErrDiscoveryRoomNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, dbinfra.ErrDiscoveryInvalid):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, dbinfra.ErrDiscoveryTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		default:
			writeError(w, http.StatusServiceUnavailable, "discovery unavailable")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}
