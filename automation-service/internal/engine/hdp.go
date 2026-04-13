package engine

import (
	"encoding/json"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

// HDPEnvelope is the common HDP v1 envelope implemented in this repo.
// See doc/hdp.md.
type HDPEnvelope = hdp.Envelope
type HDPState = hdp.State
type HDPCommandResult = hdp.CommandResult
type HDPCommand = hdp.Command

func decodeJSON[T any](b []byte) (T, error) {
	var out T
	err := json.Unmarshal(b, &out)
	return out, err
}
