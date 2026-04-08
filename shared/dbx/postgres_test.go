package dbx

import "testing"

func TestBuildPostgresDSN(t *testing.T) {
	got := BuildPostgresDSN(PostgresConfig{User: "u", Password: "p", DBName: "db", Host: "postgres", Port: "5432"})
	want := "host=postgres user=u password=p dbname=db port=5432 sslmode=disable TimeZone=UTC"
	if got != want {
		t.Fatalf("unexpected dsn: %q", got)
	}
}
