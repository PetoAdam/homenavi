module github.com/PetoAdam/homenavi/history-service

go 1.26.0

toolchain go1.26.1

require (
	github.com/PetoAdam/homenavi/shared v0.0.0
	github.com/google/uuid v1.6.0
	gorm.io/datatypes v1.2.0
	gorm.io/driver/postgres v1.5.9
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.25.11
)

require (
	github.com/eclipse/paho.mqtt.golang v1.5.0 // indirect
	github.com/go-sql-driver/mysql v1.7.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	gorm.io/driver/mysql v1.4.7 // indirect
)

replace github.com/PetoAdam/homenavi/shared => ../shared
