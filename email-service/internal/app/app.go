package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	mail "github.com/PetoAdam/homenavi/email-service/internal/email"
	httptransport "github.com/PetoAdam/homenavi/email-service/internal/http"
	smtpinfra "github.com/PetoAdam/homenavi/email-service/internal/infra/smtp"
	sharedobs "github.com/PetoAdam/homenavi/shared/observability"
)

// App is the composed email-service application.
type App struct {
	server      *http.Server
	shutdownObs func()
	logger      *slog.Logger
}

func New(cfg Config, logger *slog.Logger) *App {
	shutdownObs, promHandler, tracer := sharedobs.SetupObservability("email-service")
	sender := smtpinfra.NewSender(smtpinfra.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
		FromName: cfg.FromName, FromEmail: cfg.FromEmail,
	})
	renderer := mail.NewTemplateRenderer()
	service := mail.NewService(mail.Config{FromEmail: cfg.FromEmail, FromName: cfg.FromName, AppName: cfg.AppName}, sender, renderer)
	handler := httptransport.NewHandler(service)
	router := httptransport.NewRouter(handler, promHandler, tracer)

	return &App{
		server:      &http.Server{Addr: ":" + cfg.Port, Handler: router},
		shutdownObs: shutdownObs,
		logger:      logger,
	}
}

func (a *App) Run(ctx context.Context) error {
	defer a.shutdownObs()
	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("email service starting", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("email service shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}
