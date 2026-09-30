package demo

import (
	"testing"

	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

func TestIsDemoUserMatchesBootstrapPattern(t *testing.T) {
	user := &clientsinfra.User{
		UserName: "demo_abc123",
		Email:    "demo+abc123@example.com",
	}

	if !IsDemoUser(user) {
		t.Fatal("expected bootstrap demo user pattern to be detected")
	}
}

func TestIsDemoUserRejectsNonDemoUsers(t *testing.T) {
	tests := []*clientsinfra.User{
		nil,
		{UserName: "resident", Email: "resident@example.com"},
		{UserName: "demo_abc123", Email: "demo+abc123@demo.local"},
		{UserName: "resident", Email: "demo+abc123@example.com"},
	}

	for _, user := range tests {
		if IsDemoUser(user) {
			t.Fatalf("expected non-demo user to be ignored: %#v", user)
		}
	}
}
