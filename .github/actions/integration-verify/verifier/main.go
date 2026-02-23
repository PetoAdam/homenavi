package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`

	DeviceExtension struct {
		Enabled             bool   `json:"enabled"`
		ProviderID          string `json:"provider_id"`
		Protocol            string `json:"protocol"`
		DiscoveryMode       string `json:"discovery_mode"`
		SupportsPairing     bool   `json:"supports_pairing"`
		CapabilitySchemaURL string `json:"capability_schema_url"`
	} `json:"device_extension"`

	AutomationExtension struct {
		Enabled         bool   `json:"enabled"`
		Scope           string `json:"scope"`
		StepsCatalogURL string `json:"steps_catalog_url"`
		ExecuteEndpoint string `json:"execute_endpoint"`
	} `json:"automation_extension"`

	UI struct {
		Sidebar struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
			Label   string `json:"label"`
			Icon    string `json:"icon"`
		} `json:"sidebar"`
	} `json:"ui"`

	Widgets []struct {
		Type        string `json:"type"`
		DisplayName string `json:"display_name"`
		Entry       struct {
			Kind string `json:"kind"`
			URL  string `json:"url"`
		} `json:"entry"`
	} `json:"widgets"`
}

func main() {
	var (
		repoRoot     = flag.String("repo-root", ".", "path to the integration repo root")
		manifestPath = flag.String("manifest", "manifest/homenavi-integration.json", "path to manifest")
		schemaPath   = flag.String("schema", "homenavi-integration.schema.json", "path to schema")
	)
	flag.Parse()

	failures := 0
	fail := func(format string, args ...any) {
		failures++
		fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	}
	warn := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "WARN: "+format+"\n", args...)
	}

	rel := func(p string) string { return filepath.Join(*repoRoot, filepath.FromSlash(p)) }

	// Required files
	mustExist := []string{
		"Dockerfile",
		"manifest",
		"web/ui",
		"web/widgets",
		"manifest/homenavi-integration.json",
	}
	for _, p := range mustExist {
		if _, err := os.Stat(rel(p)); err != nil {
			fail("missing required path: %s", p)
		}
	}

	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		fail("read manifest: %v", err)
		exit(failures)
	}

	// Schema validation
	compiler := jsonschema.NewCompiler()
	schemaBytes, err := os.ReadFile(*schemaPath)
	if err != nil {
		fail("read schema: %v", err)
		exit(failures)
	}
	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaBytes))); err != nil {
		fail("load schema: %v", err)
		exit(failures)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		fail("compile schema: %v", err)
		exit(failures)
	}

	var raw any
	if err := json.Unmarshal(manifestBytes, &raw); err != nil {
		fail("manifest is not valid JSON: %v", err)
		exit(failures)
	}
	if err := schema.Validate(raw); err != nil {
		fail("manifest does not match schema: %v", err)
	}

	var m Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		fail("parse manifest: %v", err)
		exit(failures)
	}

	idRe := regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}[a-z0-9]$`)
	if !idRe.MatchString(m.ID) {
		fail("manifest id %q does not match required pattern", m.ID)
	}

	if m.DeviceExtension.Enabled {
		providerID := strings.TrimSpace(m.DeviceExtension.ProviderID)
		if providerID == "" {
			fail("device_extension.provider_id is required when device_extension.enabled=true")
		}
		protocol := strings.TrimSpace(m.DeviceExtension.Protocol)
		if protocol == "" {
			fail("device_extension.protocol is required when device_extension.enabled=true")
		} else if !idRe.MatchString(protocol) {
			fail("device_extension.protocol %q does not match required pattern", protocol)
		}
		mode := strings.ToLower(strings.TrimSpace(m.DeviceExtension.DiscoveryMode))
		if mode != "sync" && mode != "pairing" {
			fail("device_extension.discovery_mode must be one of: sync,pairing")
		}
		capURL := strings.TrimSpace(m.DeviceExtension.CapabilitySchemaURL)
		if capURL != "" && !strings.HasPrefix(capURL, "/") {
			fail("device_extension.capability_schema_url must start with '/': got %q", capURL)
		}
	}

	if m.AutomationExtension.Enabled {
		scope := strings.ToLower(strings.TrimSpace(m.AutomationExtension.Scope))
		if scope != "integration_only" {
			fail("automation_extension.scope must be 'integration_only' when automation_extension.enabled=true")
		}
		stepsCatalogURL := strings.TrimSpace(m.AutomationExtension.StepsCatalogURL)
		if stepsCatalogURL == "" {
			fail("automation_extension.steps_catalog_url is required when automation_extension.enabled=true")
		} else if !strings.HasPrefix(stepsCatalogURL, "/") {
			fail("automation_extension.steps_catalog_url must start with '/': got %q", stepsCatalogURL)
		}
		executeEndpoint := strings.TrimSpace(m.AutomationExtension.ExecuteEndpoint)
		if executeEndpoint == "" {
			fail("automation_extension.execute_endpoint is required when automation_extension.enabled=true")
		} else if !strings.HasPrefix(executeEndpoint, "/") {
			fail("automation_extension.execute_endpoint must start with '/': got %q", executeEndpoint)
		}

		catalogCandidate := filepath.Join("web", strings.TrimPrefix(stepsCatalogURL, "/"))
		catalogPath := rel(catalogCandidate)
		if _, err := os.Stat(catalogPath); err == nil {
			catalogBytes, readErr := os.ReadFile(catalogPath)
			if readErr != nil {
				fail("automation steps catalog read failed (%s): %v", catalogCandidate, readErr)
			} else {
				var catalog struct {
					Actions    []map[string]any `json:"actions"`
					Triggers   []map[string]any `json:"triggers"`
					Conditions []map[string]any `json:"conditions"`
				}
				if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
					fail("automation steps catalog must be valid JSON object (%s): %v", catalogCandidate, err)
				} else {
					checkStepKinds := func(group string, steps []map[string]any) {
						for idx, step := range steps {
							rawKind, _ := step["kind"].(string)
							kind := strings.ToLower(strings.TrimSpace(rawKind))
							if kind == "" {
								continue
							}
							if kind == "action.send_command" || kind == "trigger.device_state" {
								fail("automation steps catalog %s[%d] uses core HDP device step kind %q; keep device automations in core nodes and use integration extension only for extra features", group, idx, kind)
							}
						}
					}
					checkStepKinds("actions", catalog.Actions)
					checkStepKinds("triggers", catalog.Triggers)
					checkStepKinds("conditions", catalog.Conditions)
				}
			}
		}
	}

	// Compatibility rules (portable paths; no platform-specific prefixes)
	if m.UI.Sidebar.Enabled {
		p := strings.TrimSpace(m.UI.Sidebar.Path)
		if p == "" {
			fail("ui.sidebar.path is required when ui.sidebar.enabled=true")
		} else if strings.HasPrefix(p, "/integrations/") {
			fail("ui.sidebar.path must be portable (use /ui/... not /integrations/...): got %q", p)
		} else if !strings.HasPrefix(p, "/ui/") {
			fail("ui.sidebar.path must start with /ui/: got %q", p)
		}

		icon := strings.TrimSpace(m.UI.Sidebar.Icon)
		if strings.HasPrefix(icon, "/integrations/") {
			fail("ui.sidebar.icon must be portable (use /assets/... not /integrations/...): got %q", icon)
		} else if strings.HasPrefix(strings.ToLower(icon), "http://") || strings.HasPrefix(strings.ToLower(icon), "https://") {
			fail("ui.sidebar.icon must be bundled/same-origin (remote URLs are not allowed): %q", icon)
		}
	}

	for _, w := range m.Widgets {
		wt := strings.TrimSpace(w.Type)
		if wt == "" {
			fail("widget.type is required")
			continue
		}
		if !strings.HasPrefix(wt, "integration.") {
			warn("widget.type %q does not start with integration.", wt)
		}
		u := strings.TrimSpace(w.Entry.URL)
		if strings.HasPrefix(u, "/integrations/") {
			fail("widget.entry.url must be portable (use /widgets/... not /integrations/...): got %q", u)
		} else if !strings.HasPrefix(u, "/widgets/") {
			fail("widget.entry.url must start with /widgets/: got %q", u)
		}
		// Ensure the widget directory exists on disk (best-effort mapping)
		if strings.HasPrefix(u, "/widgets/") {
			seg := strings.TrimPrefix(u, "/widgets/")
			seg = strings.Trim(seg, "/")
			if seg != "" {
				idx := strings.Index(seg, "/")
				if idx >= 0 {
					seg = seg[:idx]
				}
				candidate := filepath.Join("web/widgets", seg, "index.html")
				if _, err := os.Stat(rel(candidate)); err != nil {
					fail("widget %q entry_url points to %q but %s is missing", wt, u, candidate)
				}
			}
		}
	}

	// Static security checks (no remote scripts/styles)
	checkHTML := func(p string) {
		b, err := os.ReadFile(p)
		if err != nil {
			return
		}
		s := string(b)
		bad := []string{
			"<script src=\"http://",
			"<script src=\"https://",
			"<link rel=\"stylesheet\" href=\"http://",
			"<link rel=\"stylesheet\" href=\"https://",
		}
		for _, needle := range bad {
			if strings.Contains(s, needle) {
				fail("%s includes remote asset reference (%s)", p, needle)
				break
			}
		}
	}

	webRoot := rel("web")
	_ = filepath.WalkDir(webRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			checkHTML(p)
		}
		return nil
	})

	if failures == 0 {
		fmt.Println("OK: integration verification passed")
	}
	exit(failures)
}

func exit(failures int) {
	if failures > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}

// keep go vet happy for older stdlib builds
var _ io.Reader
