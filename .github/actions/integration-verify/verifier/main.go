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
