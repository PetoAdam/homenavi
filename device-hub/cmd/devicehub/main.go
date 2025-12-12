package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"device-hub/internal/config"
	"device-hub/internal/httpapi"
	mqttpkg "device-hub/internal/mqtt"
	"device-hub/internal/observability"
	matterpkg "device-hub/internal/proto/matter"
	threadpkg "device-hub/internal/proto/thread"
	"device-hub/internal/store"
)

func main() {
	cfg := config.Load()
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.Postgres.Host, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.DBName, cfg.Postgres.Port)
	repo, err := store.NewRepository(dsn)
	if err != nil {
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}
	mClient := mqttpkg.New(cfg.MQTTBrokerURL)
	_ = mClient.Subscribe("homenavi/adapter/+/hello", func(_ mqtt.Client, msg mqttpkg.Message) {
		adapterTopicParts := strings.Split(msg.Topic(), "/")
		if len(adapterTopicParts) < 3 {
			return
		}
		adapterID := adapterTopicParts[2]
		var hello map[string]any
		if err := json.Unmarshal(msg.Payload(), &hello); err != nil {
			slog.Warn("hello decode failed", "error", err)
			return
		}
		ack := map[string]any{
			"type":        "hello_ack",
			"adapter_id":  adapterID,
			"hub_version": "1.4.0",
			"hdp_version": "1.0",
			"config": map[string]any{
				"state_ack_required": true,
				"command_timeout_ms": 5000,
			},
			"timestamp": time.Now().Unix(),
		}
		b, _ := json.Marshal(ack)
		topic := fmt.Sprintf("homenavi/hub/%s/hello_ack", adapterID)
		if err := mClient.PublishWith(topic, b, true); err != nil {
			slog.Warn("hello_ack publish failed", "adapter", adapterID, "error", err)
		}
	})
	_ = mClient.Subscribe("homenavi/adapter/+/status", func(_ mqtt.Client, msg mqttpkg.Message) {
		slog.Info("adapter status", "topic", msg.Topic(), "payload", string(msg.Payload()))
	})

	integrations := []httpapi.IntegrationDescriptor{
		{Protocol: "zigbee", Label: "Zigbee (Zigbee2MQTT)", Status: "active"},
	}

	matterAdapter := matterpkg.New(mClient, repo, matterpkg.Config{Enabled: cfg.EnableMatter})
	if err := matterAdapter.Start(context.Background()); err != nil {
		slog.Error("matter startup failed", "error", err)
	}
	if cfg.EnableMatter {
		integrations = append(integrations, httpapi.IntegrationDescriptor{Protocol: "matter", Label: "Matter", Status: "experimental", Notes: "adapter placeholder"})
	} else {
		integrations = append(integrations, httpapi.IntegrationDescriptor{Protocol: "matter", Label: "Matter", Status: "planned"})
	}

	threadAdapter := threadpkg.New(mClient, repo, threadpkg.Config{Enabled: cfg.EnableThread})
	if err := threadAdapter.Start(context.Background()); err != nil {
		slog.Error("thread startup failed", "error", err)
	}
	if cfg.EnableThread {
		integrations = append(integrations, httpapi.IntegrationDescriptor{Protocol: "thread", Label: "Thread", Status: "experimental", Notes: "adapter placeholder"})
	} else {
		integrations = append(integrations, httpapi.IntegrationDescriptor{Protocol: "thread", Label: "Thread", Status: "planned"})
	}

	pairingConfigs := []httpapi.PairingConfig{
		{
			Protocol:          "zigbee",
			Label:             "Zigbee (Zigbee2MQTT)",
			Supported:         true,
			SupportsInterview: true,
			DefaultTimeoutSec: 60,
			Instructions: []string{
				"Reset or power-cycle the device to enter pairing mode.",
				"Keep it close to the coordinator while pairing runs.",
				"We will auto-register it as soon as it is detected.",
			},
			CTALabel: "Start Zigbee pairing",
		},
	}
	if cfg.EnableThread {
		pairingConfigs = append(pairingConfigs, httpapi.PairingConfig{
			Protocol:          "thread",
			Label:             "Thread",
			Supported:         true,
			SupportsInterview: false,
			DefaultTimeoutSec: 60,
			Instructions: []string{
				"Ensure the Thread border router is online.",
				"Put the Thread device into commissioning mode.",
				"We will attach it when the adapter reports the join.",
			},
			CTALabel: "Start Thread pairing",
			Notes:    "Placeholder implementation",
		})
	}
	if cfg.EnableMatter {
		pairingConfigs = append(pairingConfigs, httpapi.PairingConfig{
			Protocol:          "matter",
			Label:             "Matter",
			Supported:         true,
			SupportsInterview: true,
			DefaultTimeoutSec: 120,
			Instructions: []string{
				"Open the Matter commissioning window on the device.",
				"Keep the QR code handy in case an app flow is required.",
				"We will bind the device when the adapter reports it.",
			},
			CTALabel: "Start Matter pairing",
			Notes:    "Experimental placeholder",
		})
	}

	shutdownObs, promHandler, tracer := observability.SetupObservability("device-hub")
	defer shutdownObs()

	// Only health endpoint retained for k8s / docker health checks.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	httpapi.NewServer(repo, mClient, integrations, pairingConfigs).Register(mux)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: observability.WrapHandler(tracer, "device-hub", mux)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()
	slog.Info("device-hub started", "port", cfg.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("device-hub stopped")
}
