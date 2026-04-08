package main

import "testing"

func TestNewCORSOptionsDisablesCredentialsWithWildcardOrigin(t *testing.T) {
	opts := newCORSOptions()

	if len(opts.AllowedOrigins) != 1 || opts.AllowedOrigins[0] != "*" {
		t.Fatalf("expected wildcard allowed origin, got %#v", opts.AllowedOrigins)
	}
	if opts.AllowCredentials {
		t.Fatal("expected credentials to be disabled for wildcard CORS origin")
	}
}
