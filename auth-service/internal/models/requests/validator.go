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

// Password policy (centralized):
// - Minimum length 10
// - Must include at least one lowercase, one uppercase, one digit
// - No whitespace
// - No sequence of the same character repeated 3+ times consecutively
// Returns (ok, []violations)
func passwordPolicyCheck(pw string) (bool, []string) {
	var violations []string
	if len(pw) < 10 {
		violations = append(violations, "length >= 10")
	}
	var hasLower, hasUpper, hasDigit bool
	var repeatCount int
	var lastRune rune
	for i, r := range pw {
		if r >= 'a' && r <= 'z' { hasLower = true }
		if r >= 'A' && r <= 'Z' { hasUpper = true }
		if r >= '0' && r <= '9' { hasDigit = true }
		if r == ' ' || r == '\t' || r == '\n' { violations = append(violations, "no whitespace") ; break }
		if i == 0 { lastRune = r; repeatCount = 1; continue }
		if r == lastRune { repeatCount++ } else { lastRune = r; repeatCount = 1 }
		if repeatCount == 3 { violations = append(violations, "no 3+ consecutive identical chars") ; break }
	}
	if !hasLower { violations = append(violations, "missing lowercase letter") }
	if !hasUpper { violations = append(violations, "missing uppercase letter") }
	if !hasDigit { violations = append(violations, "missing digit") }
	return len(violations) == 0, violations
}

// IsValidPassword retains original signature for existing callers; internally uses enhanced policy.
func IsValidPassword(password string) bool {
	ok, _ := passwordPolicyCheck(password)
	return ok
}

// PasswordPolicyError builds a human-friendly error string for a password that failed validation.
func PasswordPolicyError(password string) string {
	_, violations := passwordPolicyCheck(password)
	if len(violations) == 0 { return "" }
	return "password does not meet policy: " + strings.Join(violations, ", ")
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
