package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func ParseAndValidateJSON(r *http.Request, dst interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	if validator, ok := dst.(interface{ Validate() error }); ok {
		return validator.Validate()
	}

	return nil
}
