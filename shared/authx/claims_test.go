package authx

import "testing"

func TestParseScopes(t *testing.T) {
	t.Parallel()

	scopes := ParseScopes("  home.devices.read\thome.devices.write home.devices.read  ")
	if len(scopes) != 2 {
		t.Fatalf("expected two scopes, got %d", len(scopes))
	}
	if _, ok := scopes["home.devices.read"]; !ok {
		t.Fatal("expected read scope")
	}
	if _, ok := scopes["home.devices.write"]; !ok {
		t.Fatal("expected write scope")
	}
}

func TestHasScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		required string
		want     bool
	}{
		{name: "exact scope", value: "home.devices.read home.devices.write", required: "home.devices.read", want: true},
		{name: "prefix is not a match", value: "home.devices.read", required: "home.devices", want: false},
		{name: "empty required scope", value: "home.devices.read", required: "", want: false},
		{name: "missing scope", value: "home.devices.read", required: "home.automations.read", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := HasScope(test.value, test.required); got != test.want {
				t.Fatalf("HasScope(%q, %q) = %t, want %t", test.value, test.required, got, test.want)
			}
		})
	}
}

func TestHasAudience(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		audiences []string
		required  string
		want      bool
	}{
		{name: "exact audience", audiences: []string{AudienceAPI, "https://example.test/mcp"}, required: AudienceAPI, want: true},
		{name: "prefix is not a match", audiences: []string{"homenavi-api-admin"}, required: AudienceAPI, want: false},
		{name: "empty required audience", audiences: []string{AudienceAPI}, required: "", want: false},
		{name: "missing audience", audiences: []string{"https://example.test/mcp"}, required: AudienceAPI, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := HasAudience(test.audiences, test.required); got != test.want {
				t.Fatalf("HasAudience(%v, %q) = %t, want %t", test.audiences, test.required, got, test.want)
			}
		})
	}
}
