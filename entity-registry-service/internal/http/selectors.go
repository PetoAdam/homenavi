package http

import (
	"net/http"
	"strings"
)

type selectorResolveRequest struct {
	Selector string `json:"selector"`
}

type selectorResolveResponse struct {
	Selector       string   `json:"selector"`
	HDPExternalIDs []string `json:"hdp_external_ids"`
	DeviceIDs      []string `json:"device_ids"`
}

func (s *Server) handleSelectorsResolve(w http.ResponseWriter, r *http.Request) {
	var req selectorResolveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids, devIDs, err := s.repo.ResolveSelectorToHDP(r.Context(), req.Selector)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	outDev := make([]string, 0, len(devIDs))
	for _, id := range devIDs {
		outDev = append(outDev, id.String())
	}
	writeJSON(w, http.StatusOK, selectorResolveResponse{Selector: strings.TrimSpace(req.Selector), HDPExternalIDs: ids, DeviceIDs: outDev})
}
