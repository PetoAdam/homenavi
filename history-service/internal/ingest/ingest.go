package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/history-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/hdp"

	"gorm.io/datatypes"
)

var ErrNotAStateTopic = errors.New("not a state topic")

type Ingestor struct {
	Repo         *dbinfra.Repository
	StatePrefix  string
	AllowRetains bool
}

type MQTTMessage interface {
	Topic() string
	Payload() []byte
	Retained() bool
}

func (i *Ingestor) HandleMessage(ctx context.Context, msg MQTTMessage, receivedAt time.Time) {
	topic := msg.Topic()
	retained := msg.Retained()
	if retained && !i.AllowRetains {
		slog.Debug("history ingest ignoring retained", "topic", topic)
		return
	}

	deviceID, err := ParseDeviceID(i.StatePrefix, topic)
	if err != nil {
		if errors.Is(err, ErrNotAStateTopic) {
			return
		}
		slog.Warn("history ingest topic parse failed", "topic", topic, "error", err)
		return
	}

	payload := msg.Payload()
	if len(payload) == 0 {
		return
	}
	if !json.Valid(payload) {
		slog.Warn("history ingest invalid json", "topic", topic, "device_id", deviceID)
		return
	}

	p := &dbinfra.DeviceStatePoint{
		DeviceID: deviceID,
		TS:       receivedAt.UTC(),
		Payload:  datatypes.JSON(append([]byte(nil), payload...)),
		Topic:    topic,
		Retained: retained,
	}

	if err := i.Repo.InsertStatePoint(ctx, p); err != nil {
		slog.Error("history ingest db insert failed", "topic", topic, "device_id", deviceID, "error", err)
		return
	}
	slog.Debug("history state stored", "device_id", deviceID, "ts", p.TS)
}

func ParseDeviceID(prefix, topic string) (string, error) {
	if prefix == "" {
		prefix = hdp.StatePrefix
	}
	id, err := hdp.DeviceIDFromTopic(prefix, topic)
	if errors.Is(err, hdp.ErrTopicPrefixMismatch) {
		return "", ErrNotAStateTopic
	}
	return id, err
}
