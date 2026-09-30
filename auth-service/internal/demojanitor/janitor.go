package demojanitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"time"

	demohttp "github.com/PetoAdam/homenavi/auth-service/internal/http/demo"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

const activityKeyPrefix = "demo:last-active:"

type activityStore interface {
	Get(ctx context.Context, key string) (string, error)
}

type userDirectory interface {
	ListUsersInternal(values url.Values) ([]clientsinfra.User, map[string]interface{}, error)
	DeleteUserInternal(userID string) error
}

type dashboardCleaner interface {
	DeleteUserDashboard(userID string) error
}

type Config struct {
	ActivityTTL time.Duration
	HardCap     int
	PageSize    int
}

type Janitor struct {
	config     Config
	activity   activityStore
	users      userDirectory
	dashboards dashboardCleaner
	logger     *slog.Logger
}

type candidate struct {
	user       clientsinfra.User
	lastActive time.Time
	stale      bool
}

type plan struct {
	stale      []clientsinfra.User
	capVictims []clientsinfra.User
}

func New(cfg Config, activity activityStore, users userDirectory, dashboards dashboardCleaner, logger *slog.Logger) *Janitor {
	if cfg.ActivityTTL <= 0 {
		cfg.ActivityTTL = 15 * time.Minute
	}
	if cfg.HardCap <= 0 {
		cfg.HardCap = 1500
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Janitor{config: cfg, activity: activity, users: users, dashboards: dashboards, logger: logger}
}

func (j *Janitor) Cleanup(ctx context.Context) error {
	demoUsers, err := j.listDemoUsers()
	if err != nil {
		return err
	}
	candidates, err := j.loadCandidates(ctx, demoUsers)
	if err != nil {
		return err
	}
	cleanupPlan := buildPlan(time.Now(), candidates, j.config.ActivityTTL, j.config.HardCap)
	for _, user := range cleanupPlan.stale {
		j.deleteUser(user, "stale")
	}
	for _, user := range cleanupPlan.capVictims {
		j.deleteUser(user, "cap")
	}
	return nil
}

func (j *Janitor) deleteUser(user clientsinfra.User, reason string) {
	if !demohttp.IsDemoUser(&user) {
		return
	}
	if err := j.dashboards.DeleteUserDashboard(user.ID); err != nil {
		j.logger.Warn("demo janitor failed deleting dashboard", "user_id", user.ID, "reason", reason, "error", err)
		return
	}
	if err := j.users.DeleteUserInternal(user.ID); err != nil {
		j.logger.Warn("demo janitor failed deleting user", "user_id", user.ID, "reason", reason, "error", err)
		return
	}
	j.logger.Info("demo janitor deleted demo user", "user_id", user.ID, "reason", reason)
}

func (j *Janitor) listDemoUsers() ([]clientsinfra.User, error) {
	page := 1
	users := make([]clientsinfra.User, 0)
	for {
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("page_size", strconv.Itoa(j.config.PageSize))
		batch, meta, err := j.users.ListUsersInternal(values)
		if err != nil {
			return nil, fmt.Errorf("list users page %d: %w", page, err)
		}
		for _, user := range batch {
			if demohttp.IsDemoUser(&user) {
				users = append(users, user)
			}
		}
		totalPages := intMeta(meta, "total_pages")
		if totalPages <= 0 || page >= totalPages {
			return users, nil
		}
		page++
	}
}

func (j *Janitor) loadCandidates(ctx context.Context, users []clientsinfra.User) ([]candidate, error) {
	candidates := make([]candidate, 0, len(users))
	for _, user := range users {
		value, err := j.activity.Get(ctx, activityKeyPrefix+user.ID)
		if err != nil {
			if errors.Is(err, cacheinfra.ErrNotFound) {
				candidates = append(candidates, candidate{user: user, stale: true})
				continue
			}
			return nil, fmt.Errorf("load activity for %s: %w", user.ID, err)
		}
		unixSeconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			candidates = append(candidates, candidate{user: user, stale: true})
			continue
		}
		candidates = append(candidates, candidate{user: user, lastActive: time.Unix(unixSeconds, 0).UTC()})
	}
	return candidates, nil
}

func buildPlan(now time.Time, candidates []candidate, ttl time.Duration, hardCap int) plan {
	stale := make([]clientsinfra.User, 0)
	active := make([]candidate, 0, len(candidates))
	for _, item := range candidates {
		if item.stale || item.lastActive.IsZero() || now.Sub(item.lastActive) > ttl {
			stale = append(stale, item.user)
			continue
		}
		active = append(active, item)
	}
	if hardCap <= 0 || len(active) <= hardCap {
		return plan{stale: stale}
	}
	sort.Slice(active, func(i, k int) bool {
		return active[i].lastActive.Before(active[k].lastActive)
	})
	overflow := len(active) - hardCap
	capVictims := make([]clientsinfra.User, 0, overflow)
	for _, item := range active[:overflow] {
		capVictims = append(capVictims, item.user)
	}
	return plan{stale: stale, capVictims: capVictims}
}

func intMeta(meta map[string]interface{}, key string) int {
	if meta == nil {
		return 0
	}
	switch value := meta[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
