package store

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Cursor struct {
	TS time.Time
	ID uuid.UUID
}

func EncodeCursor(c Cursor) string {
	s := fmt.Sprintf("%s|%s", c.TS.UTC().Format(time.RFC3339Nano), c.ID.String())
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func DecodeCursor(v string) (*Cursor, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &Cursor{TS: ts, ID: id}, nil
}
