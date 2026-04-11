package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Service orchestrates user use cases.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (User, error) {
	if _, err := s.repo.FindByEmail(ctx, input.Email); err == nil {
		return User{}, ErrDuplicateEmail
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	if _, err := s.repo.FindByUserName(ctx, input.UserName); err == nil {
		return User{}, ErrDuplicateUserName
	} else if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}

	var passwordHash *string
	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, fmt.Errorf("hash password: %w", err)
		}
		ph := string(hash)
		passwordHash = &ph
	} else if input.GoogleID == nil || strings.TrimSpace(*input.GoogleID) == "" {
		return User{}, ErrPasswordOrGoogleIDRequired
	}

	user := User{
		ID:                 uuid.New(),
		UserName:           input.UserName,
		NormalizedUserName: strings.ToUpper(input.UserName),
		Email:              input.Email,
		NormalizedEmail:    strings.ToUpper(input.Email),
		FirstName:          input.FirstName,
		LastName:           input.LastName,
		Role:               "user",
		EmailConfirmed:     input.GoogleID != nil,
		PasswordHash:       passwordHash,
		GoogleID:           input.GoogleID,
		ProfilePictureURL:  input.ProfilePictureURL,
		TwoFactorEnabled:   false,
		LockoutEnabled:     false,
		AccessFailedCount:  0,
	}
	if err := s.repo.Create(ctx, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return User{}, err
	}
	return s.repo.FindByID(ctx, parsed)
}

func (s *Service) Lookup(ctx context.Context, email, googleID string) (User, error) {
	if strings.TrimSpace(googleID) != "" {
		return s.repo.FindByGoogleID(ctx, googleID)
	}
	return s.repo.FindByEmail(ctx, email)
}

func (s *Service) List(ctx context.Context, query string, page, size int) ([]User, int64, int, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	items, total, err := s.repo.List(ctx, strings.TrimSpace(query), page, size)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	return items, total, page, size, nil
}

func (s *Service) SetLockout(ctx context.Context, actor Actor, id string, lock bool) error {
	if actor.Role != "admin" {
		return ErrForbidden
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.repo.UpdateFields(ctx, parsed, map[string]any{"lockout_enabled": lock})
}

func (s *Service) Delete(ctx context.Context, actor Actor, id string) error {
	if !canAccessUser(actor, id) {
		return ErrForbidden
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, parsed)
}

func (s *Service) Patch(ctx context.Context, actor Actor, id string, req map[string]any) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	changingNonRole := false
	for key := range req {
		if key != "role" {
			changingNonRole = true
			break
		}
	}
	if changingNonRole && !canAccessUser(actor, id) {
		return ErrForbidden
	}

	allowed := map[string]bool{
		"email_confirmed":     true,
		"two_factor_enabled":  true,
		"two_factor_type":     true,
		"two_factor_secret":   true,
		"lockout_enabled":     true,
		"access_failed_count": true,
		"password":            true,
		"first_name":          true,
		"last_name":           true,
		"role":                true,
		"profile_picture_url": true,
		"google_id":           true,
	}

	update := make(map[string]any)
	for key, value := range req {
		if !allowed[key] {
			continue
		}
		switch key {
		case "password":
			hash, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%v", value)), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			update["password_hash"] = string(hash)
		case "role":
			newRole, ok := value.(string)
			if !ok {
				return ErrInvalidRoleValue
			}
			newRole = strings.ToLower(newRole)
			if !map[string]bool{"user": true, "resident": true, "admin": true}[newRole] {
				return ErrUnsupportedRole
			}
			switch actor.Role {
			case "admin":
				update[key] = newRole
			case "resident":
				if newRole != "resident" {
					return ErrResidentsGrantResidentOnly
				}
				target, err := s.repo.FindByID(ctx, parsed)
				if err != nil {
					return err
				}
				if target.Role == "admin" {
					return ErrCannotModifyAdminRole
				}
				update[key] = newRole
			default:
				return ErrCannotChangeRole
			}
		default:
			update[key] = value
		}
	}
	if len(update) == 0 {
		return ErrNoValidFields
	}
	return s.repo.UpdateFields(ctx, parsed, update)
}

func (s *Service) Validate(ctx context.Context, email, password string) (User, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if user.LockoutEnabled {
		return User{}, ErrAccountLocked
	}
	if user.PasswordHash == nil {
		return User{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

func canAccessUser(actor Actor, userID string) bool {
	return actor.Role == "admin" || actor.Subject == userID
}
