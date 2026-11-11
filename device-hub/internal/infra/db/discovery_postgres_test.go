package db

import (
	"context"
	"os"
	"testing"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMetadataRevisionPostgres(t *testing.T) {
	dsn := os.Getenv("DISCOVERY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DISCOVERY_TEST_POSTGRES_DSN not set")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureSchema(database); err != nil {
		t.Fatal(err)
	}
	repo := New(database)
	ctx := context.Background()
	initial, err := repo.MetadataRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	device := model.Device{Protocol: "thread", ExternalID: "discovery-postgres-test", Type: "sensor", Capabilities: datatypes.JSON(`[]`)}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	created, err := repo.MetadataRevision(ctx)
	if err != nil || created <= initial {
		t.Fatalf("initial=%d created=%d err=%v", initial, created, err)
	}
	if err := database.Model(&device).Update("online", true).Error; err != nil {
		t.Fatal(err)
	}
	unchanged, err := repo.MetadataRevision(ctx)
	if err != nil || unchanged != created {
		t.Fatalf("online update changed revision: %d %d %v", created, unchanged, err)
	}
	if err := database.Model(&device).Update("type", "thermostat").Error; err != nil {
		t.Fatal(err)
	}
	changed, err := repo.MetadataRevision(ctx)
	if err != nil || changed <= unchanged {
		t.Fatalf("metadata update revision: %d %d %v", unchanged, changed, err)
	}
	if err := database.Delete(&device).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.MetadataRevision(ctx)
	if err != nil || deleted <= changed {
		t.Fatalf("delete revision: %d %d %v", changed, deleted, err)
	}
}
