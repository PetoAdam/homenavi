package http

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/PetoAdam/homenavi/shared/envx"
)

func (s *Server) composeEnabled() bool {
	val := envx.String("INTEGRATIONS_COMPOSE_ENABLED", "")
	if val != "" {
		val = strings.ToLower(val)
		return val == "1" || val == "true" || val == "yes"
	}
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return true
	}
	return false
}

func (s *Server) composeBaseArgs(fileOverride string) ([]string, error) {
	file := strings.TrimSpace(fileOverride)
	if file == "" {
		file = envx.String("INTEGRATIONS_COMPOSE_FILE", "")
	}
	if file == "" {
		file = envx.String("DOCKER_COMPOSE_FILE", "")
	}
	project := envx.String("INTEGRATIONS_COMPOSE_PROJECT", "")
	if file == "" {
		return nil, fmt.Errorf("compose file not configured")
	}
	args := []string{"compose", "-f", file}
	if envFile := envx.String("INTEGRATIONS_COMPOSE_ENV_FILE", ""); envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	if project != "" {
		args = append(args, "-p", project)
	}
	return args, nil
}

func (s *Server) expandComposePath(pathRaw string) string {
	pathRaw = strings.TrimSpace(pathRaw)
	if pathRaw == "" {
		return ""
	}
	root := envx.String("INTEGRATIONS_ROOT", "")
	if root == "" {
		return pathRaw
	}
	replaced := strings.ReplaceAll(pathRaw, "${INTEGRATIONS_ROOT}", root)
	if replaced == pathRaw {
		replaced = strings.ReplaceAll(pathRaw, "$INTEGRATIONS_ROOT", root)
	}
	return replaced
}

type composeSpec struct {
	Services map[string]any `yaml:"services"`
}

func (s *Server) composeServiceForID(composeFile, id string) (string, error) {
	service := strings.TrimSpace(id)
	if strings.TrimSpace(composeFile) == "" {
		return service, nil
	}
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return service, nil
	}
	var spec composeSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return service, nil
	}
	if len(spec.Services) == 0 {
		return service, nil
	}
	if _, ok := spec.Services[service]; ok {
		return service, nil
	}
	if len(spec.Services) == 1 {
		for name := range spec.Services {
			return name, nil
		}
	}
	serviceNames := make([]string, 0, len(spec.Services))
	for name := range spec.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	return "", fmt.Errorf("compose file has multiple services and none match integration id %q (services: %s)", service, strings.Join(serviceNames, ", "))
}

func defaultUpstreamForID(id string) string {
	safeID := strings.TrimSpace(id)
	if safeID == "" || !regexp.MustCompile(`^[a-z0-9._-]+$`).MatchString(safeID) {
		return ""
	}
	return fmt.Sprintf("http://%s:8099", safeID)
}

func (s *Server) runCompose(ctx context.Context, composeFile string, args ...string) error {
	if !s.composeEnabled() {
		return nil
	}
	base, err := s.composeBaseArgs(composeFile)
	if err != nil {
		return err
	}
	bin := envx.String("INTEGRATIONS_COMPOSE_BIN", "")
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.CommandContext(ctx, bin, append(base, args...)...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Server) composePullTimeout() time.Duration {
	raw := envx.String("INTEGRATIONS_COMPOSE_PULL_TIMEOUT", "")
	if raw == "" {
		return 2 * time.Minute
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 2 * time.Minute
	}
	if parsed <= 0 {
		return 2 * time.Minute
	}
	return parsed
}

func (s *Server) operationTimeout() time.Duration {
	raw := envx.String("INTEGRATIONS_OPERATION_TIMEOUT", "")
	if raw == "" {
		return 15 * time.Minute
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 15 * time.Minute
	}
	if parsed < 30*time.Second {
		return 30 * time.Second
	}
	if parsed > 2*time.Hour {
		return 2 * time.Hour
	}
	return parsed
}

func (s *Server) composeInstall(ctx context.Context, composeFile, id string) error {
	if !s.composeEnabled() {
		return nil
	}
	service, err := s.composeServiceForID(composeFile, id)
	if err != nil {
		return err
	}
	s.setInstallStatus(id, "pulling", 35, "Pulling container image")
	pullCtx, cancel := context.WithTimeout(ctx, s.composePullTimeout())
	defer cancel()
	if err := s.runCompose(pullCtx, composeFile, "pull", service); err != nil {
		return err
	}
	s.setInstallStatus(id, "starting", 55, "Starting container")
	return s.runCompose(ctx, composeFile, "up", "-d", service)
}

func (s *Server) composeUninstall(ctx context.Context, composeFile, id string) error {
	if !s.composeEnabled() {
		return nil
	}
	service, err := s.composeServiceForID(composeFile, id)
	if err != nil {
		return err
	}
	if err := s.runCompose(ctx, composeFile, "stop", service); err != nil {
		return err
	}
	return s.runCompose(ctx, composeFile, "rm", "-f", service)
}
