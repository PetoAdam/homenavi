package requests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func ValidateRequired(fields map[string]string) error {
	var missing []string
	for field, value := range fields {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

func IsValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

func IsValidPassword(password string) bool {
	if len(password) < 8 {
		return false
	}

	var hasLower, hasUpper, hasDigit bool
	for _, c := range password {
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}

	return hasLower && hasUpper && hasDigit
}

func IsValidUserName(username string) bool {
	if len(username) < 3 || len(username) > 50 {
		return false
	}
	// Allow alphanumeric, underscore, and hyphen
	re := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return re.MatchString(username)
}

func IsValidName(name string) bool {
	if len(name) < 1 || len(name) > 100 {
		return false
	}
	// Allow letters, spaces, apostrophes, and hyphens
	re := regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	return re.MatchString(name)
}

func IsValidUUID(uuid string) bool {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	return re.MatchString(uuid)
}

func IsValidCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	// Only digits
	re := regexp.MustCompile(`^[0-9]{6}$`)
	return re.MatchString(code)
}

// Helper to parse and validate JSON request
func ParseAndValidateJSON(r *http.Request, dst interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	if validator, ok := dst.(interface{ Validate() error }); ok {
		return validator.Validate()
	}

	return nil
}
