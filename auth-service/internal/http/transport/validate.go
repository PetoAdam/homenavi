package transport

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	emailRe    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	nameRe     = regexp.MustCompile(`^[a-zA-Z\s'-]+$`)
	uuidRe     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codeRe     = regexp.MustCompile(`^[0-9]{6}$`)
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
	return emailRe.MatchString(email)
}

func passwordPolicyCheck(password string) (bool, []string) {
	var violations []string
	if len(password) < 10 {
		violations = append(violations, "length >= 10")
	}

	var hasLower, hasUpper, hasDigit bool
	var repeatCount int
	var lastRune rune
	for i, r := range password {
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == ' ' || r == '\t' || r == '\n' {
			violations = append(violations, "no whitespace")
			break
		}
		if i == 0 {
			lastRune = r
			repeatCount = 1
			continue
		}
		if r == lastRune {
			repeatCount++
		} else {
			lastRune = r
			repeatCount = 1
		}
		if repeatCount == 3 {
			violations = append(violations, "no 3+ consecutive identical chars")
			break
		}
	}
	if !hasLower {
		violations = append(violations, "missing lowercase letter")
	}
	if !hasUpper {
		violations = append(violations, "missing uppercase letter")
	}
	if !hasDigit {
		violations = append(violations, "missing digit")
	}
	return len(violations) == 0, violations
}

func IsValidPassword(password string) bool {
	ok, _ := passwordPolicyCheck(password)
	return ok
}

func PasswordPolicyError(password string) string {
	_, violations := passwordPolicyCheck(password)
	if len(violations) == 0 {
		return ""
	}
	return "password does not meet policy: " + strings.Join(violations, ", ")
}

func IsValidUserName(username string) bool {
	if len(username) < 3 || len(username) > 50 {
		return false
	}
	return usernameRe.MatchString(username)
}

func IsValidName(name string) bool {
	if len(name) < 1 || len(name) > 100 {
		return false
	}
	return nameRe.MatchString(name)
}

func IsValidUUID(value string) bool {
	return uuidRe.MatchString(value)
}

func IsValidCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	return codeRe.MatchString(code)
}
