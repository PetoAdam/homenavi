import React from 'react';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassPill from '../common/GlassPill/GlassPill';
import ChipMultiSelect from '../common/ChipMultiSelect/ChipMultiSelect';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowUpRightFromSquare,
  faCircleInfo,
  faClock,
  faHouse,
  faSignal,
  faSliders,
  faTrash,
  faXmark,
} from '@fortawesome/free-solid-svg-icons';

export default function MapDevicePopover({
  expandedDevice,
  editEnabled,
  favoritesEditorKey,
  setFavoritesEditorKey,
  removeDeviceFromMap,
  persistDeviceFavoriteFields,
  setExpandedDeviceKey,
  navigate,
  iconForFactLabel,
}) {
  if (!expandedDevice || !Number.isFinite(expandedDevice.screenX) || !Number.isFinite(expandedDevice.screenY)) return null;

  return (
    <div
      className="map-device-popover"
      style={{ left: expandedDevice.screenX, top: expandedDevice.screenY }}
      onPointerDown={(e) => e.stopPropagation()}
      onPointerUp={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
    >
      <GlassCard interactive={false} className="map-popover-card">
        <div className="map-popover-scroll">
          <div className="map-popover-header">
            <div className="map-popover-title" title={expandedDevice.displayName}>
              {expandedDevice.displayName}
            </div>
            <div className="map-popover-actions">
              <button
                type="button"
                className="map-popover-icon-btn"
                onClick={() => navigate(`/devices/${encodeURIComponent(expandedDevice.deviceRouteId)}`)}
                aria-label="Open device"
                title="Open device"
              >
                <FontAwesomeIcon icon={faArrowUpRightFromSquare} />
              </button>
              {editEnabled ? (
                <>
                  <button
                    type="button"
                    className={`map-popover-icon-btn${favoritesEditorKey === expandedDevice.key ? ' active' : ''}`}
                    onClick={() => setFavoritesEditorKey(prev => (prev === expandedDevice.key ? '' : expandedDevice.key))}
                    aria-label="Edit favorite fields"
                    title="Edit favorite fields"
                  >
                    <FontAwesomeIcon icon={faSliders} />
                  </button>
                  <button
                    type="button"
                    className="map-popover-icon-btn"
                    onClick={() => removeDeviceFromMap(expandedDevice.key)}
                    aria-label="Remove from map"
                    title="Remove from map"
                  >
                    <FontAwesomeIcon icon={faTrash} />
                  </button>
                </>
              ) : null}
              <button
                type="button"
                className="map-popover-icon-btn"
                onClick={() => setExpandedDeviceKey('')}
                aria-label="Close"
                title="Close"
              >
                <FontAwesomeIcon icon={faXmark} />
              </button>
            </div>
          </div>

          <div className="map-popover-subrow">
            <GlassPill icon={faSignal} text={expandedDevice.online ? 'Online' : 'Offline'} tone={expandedDevice.online ? 'success' : 'danger'} />
            {expandedDevice.lastSeenText ? (
              <span className="map-popover-meta" title={`Last seen ${expandedDevice.lastSeenText}`}>
                <FontAwesomeIcon icon={faClock} />
                <span>{expandedDevice.lastSeenText}</span>
              </span>
            ) : null}
            {expandedDevice.roomName ? (
              <span className="map-popover-meta" title={`Room: ${expandedDevice.roomName}`}>
                <FontAwesomeIcon icon={faHouse} />
                <span>{expandedDevice.roomName}</span>
              </span>
            ) : null}
          </div>

          {expandedDevice.facts.length ? (
            <div className="map-popover-facts" aria-label="Device highlights">
              {expandedDevice.facts.map(f => {
                const icon = iconForFactLabel(f.key || f.label);
                const title = `${f.label}: ${f.value}`;
                const showKey = icon === faCircleInfo;
                const text = showKey ? `${f.label}: ${f.value}` : `${f.value}`;
                return (
                  <GlassPill
                    key={f.label}
                    icon={icon}
                    text={text}
                    tone={f.favorite ? 'success' : 'default'}
                    title={title}
                  />
                );
              })}
            </div>
          ) : null}

          {expandedDevice.tags.length ? (
            <div className="automation-chip-row map-popover-tags" aria-label="Tags">
              {expandedDevice.tags.map(t => (
                <span key={t} className="automation-chip"><span className="subtle">#</span>{t}</span>
              ))}
            </div>
          ) : null}

          {editEnabled && favoritesEditorKey === expandedDevice.key ? (
            <div className="map-popover-editor" aria-label="Favorite fields">
              <div className="map-popover-editor-title">
                <span>Favorite fields</span>
              </div>
              <ChipMultiSelect
                ariaLabel="Favorite fields"
                options={expandedDevice.favoriteOptions}
                value={Array.isArray(expandedDevice.favoriteFields) ? expandedDevice.favoriteFields : []}
                onChange={(selected) => {
                  void persistDeviceFavoriteFields(expandedDevice.key, selected);
                }}
              />
            </div>
          ) : null}
        </div>
      </GlassCard>
    </div>
  );
}
