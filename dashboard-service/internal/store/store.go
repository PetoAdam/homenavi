package store

import (
	"github.com/PetoAdam/homenavi/shared/dbx"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func OpenPostgres(user, pass, dbName, host, port, sslmode string) (*gorm.DB, error) {
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{Host: host, Port: port, User: user, Password: pass, DBName: dbName, SSLMode: sslmode})
	return gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
}
