package db

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) FindDeviceIDByHDPExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error) {
	x := strings.TrimSpace(externalID)
	if x == "" {
		return uuid.Nil, false, nil
	}
	var binding DeviceBinding
	err := r.db.WithContext(ctx).Select("device_id").Where("kind = ? AND external_id = ?", "hdp", x).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	if binding.DeviceID == uuid.Nil {
		return uuid.Nil, false, nil
	}
	return binding.DeviceID, true, nil
}

func (r *Repository) EnsureDeviceForHDP(ctx context.Context, externalID, name, description string) (uuid.UUID, bool, error) {
	x := strings.TrimSpace(externalID)
	if x == "" {
		return uuid.Nil, false, errors.New("external_id is required")
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = x
	}
	trimmedDesc := strings.TrimSpace(description)

	var outID uuid.UUID
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var binding DeviceBinding
		err := tx.Select("device_id").Where("kind = ? AND external_id = ?", "hdp", x).First(&binding).Error
		if err == nil {
			outID = binding.DeviceID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		dev := &Device{ID: uuid.New(), Name: trimmedName, Description: trimmedDesc}
		if err := tx.Create(dev).Error; err != nil {
			return err
		}
		b := &DeviceBinding{ID: uuid.New(), DeviceID: dev.ID, Kind: "hdp", ExternalID: x}
		if err := tx.Create(b).Error; err != nil {
			return err
		}
		outID = dev.ID
		created = true
		return nil
	})
	if err != nil {
		if id, ok, lookupErr := r.FindDeviceIDByHDPExternalID(ctx, x); lookupErr == nil && ok {
			return id, false, nil
		}
		return uuid.Nil, false, err
	}
	return outID, created, nil
}

func (r *Repository) SetDeviceHDPBindings(ctx context.Context, deviceID uuid.UUID, externalIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ? AND kind = ?", deviceID, "hdp").Delete(&DeviceBinding{}).Error; err != nil {
			return err
		}
		seen := map[string]struct{}{}
		rows := make([]DeviceBinding, 0, len(externalIDs))
		for _, raw := range externalIDs {
			x := strings.TrimSpace(raw)
			if x == "" {
				continue
			}
			if _, ok := seen[x]; ok {
				continue
			}
			seen[x] = struct{}{}
			rows = append(rows, DeviceBinding{ID: uuid.New(), DeviceID: deviceID, Kind: "hdp", ExternalID: x})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}
