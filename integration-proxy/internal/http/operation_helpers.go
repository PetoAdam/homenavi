package http

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func startRuntimeOperation(parent context.Context, name string, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(parent))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	ctx, span := otel.Tracer("homenavi/integration-proxy").Start(ctx, name)
	return ctx, func() {
		span.End()
		cancel()
	}
}

func startBackgroundRuntimeOperation(parent trace.SpanContext, name string) (context.Context, trace.Span) {
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	return otel.Tracer("homenavi/integration-proxy").Start(ctx, name)
}

type integrationOperation struct {
	s    *Server
	kind string
	id   string
}

func newIntegrationOperation(s *Server, kind, id string) integrationOperation {
	return integrationOperation{
		s:    s,
		kind: strings.TrimSpace(kind),
		id:   strings.TrimSpace(id),
	}
}

func (op integrationOperation) set(stage string, progress int, message string) {
	op.s.setInstallStatus(op.id, stage, progress, message)
	op.s.logger.Printf("integration %s id=%s stage=%s progress=%d message=%q", op.kind, op.id, stage, progress, strings.TrimSpace(message))
}

func (op integrationOperation) fail(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = fmt.Sprintf("%s failed", op.kind)
	}
	op.set("error", 100, msg)
	op.s.logger.Printf("integration %s id=%s failed err=%v", op.kind, op.id, err)
	return err
}

func (op integrationOperation) done(message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "Completed"
	}
	op.set("ready", 100, msg)
	op.s.logger.Printf("integration %s id=%s completed", op.kind, op.id)
}

func (s *Server) resolveComposeFileForOperation(id, requestedComposeFile, requestedVersion string) (string, error) {
	composeFile, err := s.resolveComposeFileFromPayload(id, requestedComposeFile)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(composeFile) == "" {
		composeFile = s.defaultComposeFile(id)
	}
	composeFile = s.expandComposePath(composeFile)
	if strings.TrimSpace(composeFile) == "" {
		return "", nil
	}
	return s.materializeComposeFileForVersion(id, composeFile, requestedVersion)
}

func (s *Server) materializeComposeFileForVersion(id, composeFile, requestedVersion string) (string, error) {
	composeFile = strings.TrimSpace(composeFile)
	requestedVersion = strings.TrimSpace(requestedVersion)
	if composeFile == "" || requestedVersion == "" {
		return composeFile, nil
	}
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\$\{HN_VERSION(?:(?::-|-)[^}]*)?\}`)
	rewritten := re.ReplaceAll(data, []byte(requestedVersion))
	if bytes.Equal(rewritten, data) {
		return composeFile, nil
	}
	baseDir := filepath.Dir(composeFile)
	if strings.TrimSpace(s.configPath) != "" {
		baseDir = filepath.Join(filepath.Dir(s.configPath), "compose")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	resolvedPath := filepath.Join(baseDir, fmt.Sprintf("%s.yml", strings.TrimSpace(id)))
	if err := os.WriteFile(resolvedPath, append(rewritten, '\n'), 0o644); err != nil {
		return "", err
	}
	return resolvedPath, nil
}
