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
	HDPDeviceIDs   []string `json:"hdp_device_ids"`
	DeviceIDs      []string `json:"device_ids"`
}

func (s *Server) handleSelectorsResolve(w http.ResponseWriter, r *http.Request) {
	var req selectorResolveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	targets, devIDs, err := s.repo.ResolveSelectorToHDPTargets(r.Context(), req.Selector)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]string, 0, len(targets))
	hdpDeviceIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ExternalID)
		if target.HDPDeviceID != nil {
			hdpDeviceIDs = append(hdpDeviceIDs, target.HDPDeviceID.String())
		}
	}
	outDev := make([]string, 0, len(devIDs))
	for _, id := range devIDs {
		outDev = append(outDev, id.String())
	}
	writeJSON(w, http.StatusOK, selectorResolveResponse{Selector: strings.TrimSpace(req.Selector), HDPExternalIDs: ids, HDPDeviceIDs: hdpDeviceIDs, DeviceIDs: outDev})
}
