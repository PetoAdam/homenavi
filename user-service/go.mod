module user-service

go 1.22

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/lib/pq v1.10.9 // for Postgres
	gorm.io/driver/postgres v1.5.2
	gorm.io/gorm v1.25.6
)

require (
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	golang.org/x/crypto v0.8.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.3.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/text v0.9.0 // indirect
)
