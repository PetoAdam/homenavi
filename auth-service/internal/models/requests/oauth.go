package requests

import (
	"fmt"
	"strings"
)

type GoogleOAuthRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

func (r *GoogleOAuthRequest) Validate() error {
	var errs []string

	if r.Code == "" {
		errs = append(errs, "code is required")
	}

	if r.RedirectURI == "" {
		errs = append(errs, "redirect_uri is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errs, ", "))
	}

	return nil
}
