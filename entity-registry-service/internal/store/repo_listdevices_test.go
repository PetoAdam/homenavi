package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type queryCountingLogger struct {
	mu sync.Mutex
	n  int
}

func (l *queryCountingLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (l *queryCountingLogger) Info(context.Context, string, ...any)      {}
func (l *queryCountingLogger) Warn(context.Context, string, ...any)      {}
func (l *queryCountingLogger) Error(context.Context, string, ...any)     {}
func (l *queryCountingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.mu.Lock()
	l.n++
	l.mu.Unlock()
	_ = ctx
	_ = begin
	_ = fc
	_ = err
}

func (l *queryCountingLogger) Reset() {
	l.mu.Lock()
	l.n = 0
	l.mu.Unlock()
}

func (l *queryCountingLogger) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n
}

func TestListDevices_IsNotNPlusOne(t *testing.T) {
	ql := &queryCountingLogger{}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{Logger: ql})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()

	// Create tags.
	tag1 := &Tag{ID: uuid.New(), Slug: "kitchen", Name: "Kitchen"}
	tag2 := &Tag{ID: uuid.New(), Slug: "lights", Name: "Lights"}
	if err := db.WithContext(ctx).Create(tag1).Error; err != nil {
		t.Fatalf("create tag1: %v", err)
	}
	if err := db.WithContext(ctx).Create(tag2).Error; err != nil {
		t.Fatalf("create tag2: %v", err)
	}

	// Create multiple devices with tags and multiple bindings.
	for i := 0; i < 10; i++ {
		dev := &Device{ID: uuid.New(), Name: "Dev"}
		if err := repo.CreateDevice(ctx, dev); err != nil {
			t.Fatalf("create device: %v", err)
		}
		_ = repo.SetDeviceTags(ctx, dev.ID, []uuid.UUID{tag1.ID, tag2.ID})
		_ = repo.SetDeviceHDPBindings(ctx, dev.ID, []string{"zigbee/" + uuid.NewString(), "thread/" + uuid.NewString()})
	}

	ql.Reset()
	_, err = repo.ListDevices(ctx)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}

	// Expect a small constant number of queries, not linear with device count.
	// Current implementation should be ~4 queries (devices, device_tags, tags, bindings).
	if got := ql.Count(); got > 6 {
		t.Fatalf("expected ListDevices to run <= 6 queries, got %d", got)
	}
}
