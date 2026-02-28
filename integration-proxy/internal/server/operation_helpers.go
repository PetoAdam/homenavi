package server

import (
	"fmt"
	"strings"
)

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

func (s *Server) resolveComposeFileForOperation(id, requestedComposeFile string) (string, error) {
	composeFile, err := s.resolveComposeFileFromPayload(id, requestedComposeFile)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(composeFile) == "" {
		composeFile = s.defaultComposeFile(id)
	}
	composeFile = s.expandComposePath(composeFile)
	return composeFile, nil
}
