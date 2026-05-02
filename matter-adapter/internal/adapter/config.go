package adapter

import "time"

// Config controls adapter runtime behavior.
type Config struct {
	Enabled                  bool
	AdapterID                string
	Version                  string
	DefaultTimeoutSec        int
	DefaultNetworkPath       string
	CommissioningInterface   string
	EnableBLE                bool
	EnableThread             bool
	EnableOnNetwork          bool
	OTBRBaseURL              string
	OTBRExpectedState        string
	ThreadBorderRouterHost   string
	ThreadBorderRouterPort   int
	ThreadDatasetSource      string
	ThreadOperationalDataset string
	CommissionerEnabled      bool
	CommissionerCommand      string
	CommissionerArgs         []string
	CommissionerTimeout      time.Duration
}
