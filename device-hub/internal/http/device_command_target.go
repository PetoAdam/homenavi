package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

func (s *Server) handleDeviceCommandTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/hdp/device-command-targets/")
	if _, ok := deviceStateKeys([]string{deviceID}); !ok {
		http.Error(w, "canonical device ID required", http.StatusBadRequest)
		return
	}
	device, canonical, err := s.resolveDevice(r.Context(), deviceID)
	if err != nil {
		http.Error(w, "device lookup unavailable", http.StatusServiceUnavailable)
		return
	}
	if device == nil || canonical != deviceID {
		http.NotFound(w, r)
		return
	}
	interfaces := hdp.DiscoverCommandInterfaces(json.RawMessage(device.Capabilities), json.RawMessage(device.Inputs))
	writable := make([]hdp.CommandInterface, 0, len(interfaces.Commands))
	for _, command := range interfaces.Commands {
		if command.Access == "writable" {
			command.Readable = nil
			writable = append(writable, command)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"device_id": canonical, "online": device.Online, "metadata_status": interfaces.Status, "commands": writable})
}
