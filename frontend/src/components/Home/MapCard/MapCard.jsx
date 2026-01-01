import React, { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { faChevronRight, faMap } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../../common/GlassCard/GlassCard';
import GlassPill from '../../common/GlassPill/GlassPill';
import NoPermissionWidget from '../../common/NoPermissionWidget/NoPermissionWidget';
import { useAuth } from '../../../context/AuthContext';
import useDeviceHubDevices from '../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../hooks/useErsInventory';
import { mergeDevicePlacementsFromErs, mergeRoomsFromErs } from '../../Map/mapHydrate';
import { roomPolygonToPath } from '../../Map/mapGeometry';
import { readFavoriteFieldsFromErsMeta } from '../../Map/mapErsMeta';
import { formatMetricValueAndUnitForKey } from '../../../utils/stateFormat';
import { getFaSvgPath, iconForFactLabel, pickStateValue } from '../../Map/mapDeviceUtils';
import './MapCard.css';

function safeString(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function computeBounds({ rooms, placements }) {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  (Array.isArray(rooms) ? rooms : []).forEach((r) => {
    (Array.isArray(r?.points) ? r.points : []).forEach((p) => {
      if (!Number.isFinite(p?.x) || !Number.isFinite(p?.y)) return;
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x);
      maxY = Math.max(maxY, p.y);
    });
  });

  Object.values(placements || {}).forEach((pl) => {
    const x = Number(pl?.x);
    const y = Number(pl?.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x);
    maxY = Math.max(maxY, y);
  });

  if (!Number.isFinite(minX) || !Number.isFinite(minY) || !Number.isFinite(maxX) || !Number.isFinite(maxY)) {
    return null;
  }

  const pad = 24;
  return {
    minX: minX - pad,
    minY: minY - pad,
    maxX: maxX + pad,
    maxY: maxY + pad,
  };
}

export default function MapCard() {
  const navigate = useNavigate();
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const {
    devices: realtimeDevices,
    loading: realtimeLoading,
  } = useDeviceHubDevices({ enabled: Boolean(isResidentOrAdmin), metadataMode: 'ws' });

  const { devices, rooms, loading, error } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const layoutRooms = useMemo(() => {
    return mergeRoomsFromErs({ rooms: [] }, rooms).rooms || [];
  }, [rooms]);

  const placements = useMemo(() => {
    return mergeDevicePlacementsFromErs({ devicePlacements: {} }, devices, '').devicePlacements || {};
  }, [devices]);

  const deviceByKey = useMemo(() => {
    const m = new Map();
    (Array.isArray(devices) ? devices : []).forEach((d) => {
      const ersId = safeString(d?.ersId);
      const id = safeString(d?.id);
      const hdpId = safeString(d?.hdpId);
      if (ersId && !m.has(ersId)) m.set(ersId, d);
      if (id && !m.has(id)) m.set(id, d);
      if (hdpId && !m.has(hdpId)) m.set(hdpId, d);
    });
    return m;
  }, [devices]);

  const bounds = useMemo(() => computeBounds({ rooms: layoutRooms, placements }), [layoutRooms, placements]);

  const canNavigate = Boolean(isResidentOrAdmin);
  const handleOpenMap = () => {
    if (!canNavigate) return;
    navigate('/map');
  };

  const previewLoading = Boolean(bootstrapping || loading || realtimeLoading);

  return (
    <GlassCard
      className="map-card"
      interactive={canNavigate}
      onClick={handleOpenMap}
      role={canNavigate ? 'button' : undefined}
      tabIndex={canNavigate ? 0 : undefined}
      onKeyDown={(e) => {
        if (!canNavigate) return;
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleOpenMap();
        }
      }}
    >
      <div className="map-card-inner">
        <div className="map-card-header">
          <div className="map-card-header-left">
            <div className="map-card-title">Map</div>
            <div className="map-card-subtitle">Rooms & devices</div>
          </div>
          {canNavigate ? (
            <div className="map-card-header-right" onClick={(e) => e.stopPropagation()}>
              <GlassPill
                icon={faChevronRight}
                text="Open"
                tone="default"
                onClick={handleOpenMap}
                className="map-card-open-pill"
              />
            </div>
          ) : null}
        </div>

        <div className="map-card-preview" aria-label="Apartment map preview">
          {!isResidentOrAdmin ? (
            <NoPermissionWidget
              title="Map"
              message="Sign in with a resident account to view your map preview."
              showLogin
            />
          ) : null}

          {isResidentOrAdmin && previewLoading ? (
            <div className="map-card-loading" aria-label="Loading map preview">
              <span className="map-card-spinner" aria-hidden="true" />
              <div className="map-card-loading-text">Loading map…</div>
            </div>
          ) : null}

          {isResidentOrAdmin && !previewLoading && error ? (
            <div className="map-card-empty">
              <div className="map-card-empty-title">Could not load map</div>
              <div className="map-card-empty-sub muted">{error}</div>
            </div>
          ) : null}

          {isResidentOrAdmin && !previewLoading && !error ? (
            bounds ? (
              <svg
                className="map-card-svg"
                viewBox={`${bounds.minX} ${bounds.minY} ${bounds.maxX - bounds.minX} ${bounds.maxY - bounds.minY}`}
                preserveAspectRatio="xMidYMid meet"
              >
                <defs>
                  <linearGradient id="mapCardRoomFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="rgba(255,255,255,0.10)" />
                    <stop offset="100%" stopColor="rgba(255,255,255,0.02)" />
                  </linearGradient>
                </defs>
                <rect x={bounds.minX} y={bounds.minY} width={bounds.maxX - bounds.minX} height={bounds.maxY - bounds.minY} className="map-card-svg-bg" />
                <g className="map-card-rooms">
                  {layoutRooms.map((r) => (
                    <path key={r.id} d={roomPolygonToPath(r.points)} className="map-card-room" />
                  ))}
                </g>
                <g className="map-card-devices">
                  {Object.entries(placements).map(([k, pl]) => {
                    const x = Number(pl?.x);
                    const y = Number(pl?.y);
                    if (!Number.isFinite(x) || !Number.isFinite(y)) return null;

                    const dev = deviceByKey.get(k);
                    const label = safeString(dev?.displayName || dev?.name || k);
                    const state = dev?.state && typeof dev.state === 'object' ? dev.state : null;
                    const favorites = dev ? readFavoriteFieldsFromErsMeta(dev) : [];
                    const allCaps = [
                      ...(Array.isArray(dev?.capabilities) ? dev.capabilities : []),
                      ...(Array.isArray(dev?.state?.capabilities) ? dev.state.capabilities : []),
                    ];
                    const favoriteLines = favorites
                      .map((favoriteKey) => {
                        const raw = favoriteKey && state ? pickStateValue(state, favoriteKey) : undefined;
                        const { valueText, unit } = formatMetricValueAndUnitForKey(favoriteKey, raw, allCaps);
                        const text = valueText ? `${valueText}${unit || ''}` : '';
                        if (!text) return null;
                        return {
                          key: favoriteKey,
                          text,
                          icon: iconForFactLabel(favoriteKey),
                        };
                      })
                      .filter(Boolean);

                    return (
                      <g key={`dev-${k}`} className="map-card-device-group">
                        <circle cx={x} cy={y} r={6} className="map-card-device" />
                        {label ? (
                          <text x={x + 10} y={y + 5} className="map-card-device-name">{label}</text>
                        ) : null}
                        {favoriteLines.length ? (
                          <>
                            {favoriteLines.map((line, idx) => {
                              const path = line.icon ? getFaSvgPath(line.icon) : null;
                              const iconX = x + 10;
                              const iconY = y + 12 + (idx * 16);
                              const textX = x + 10 + 16;
                              const textY = y + 24 + (idx * 16);
                              return (
                                <g key={`${k}-fav-${line.key}-${idx}`} className="map-card-device-fav-row">
                                  {path ? (
                                    <g transform={`translate(${iconX} ${iconY}) scale(${12 / path.height})`} aria-hidden="true">
                                      <path className="map-card-device-fav-icon" d={path.path} />
                                    </g>
                                  ) : null}
                                  <text x={textX} y={textY} className="map-card-device-fav">{line.text}</text>
                                </g>
                              );
                            })}
                          </>
                        ) : null}
                      </g>
                    );
                  })}
                </g>
              </svg>
            ) : (
              <div className="map-card-empty">
                <div className="map-card-empty-title">No map yet</div>
                <div className="map-card-empty-sub muted">Draw rooms on the Map page to see a preview here.</div>
                {canNavigate ? (
                  <div className="map-card-empty-actions" onClick={(e) => e.stopPropagation()}>
                    <GlassPill
                      icon={faMap}
                      text="Go to Map"
                      tone="success"
                      onClick={handleOpenMap}
                      className="map-card-open-pill"
                    />
                  </div>
                ) : null}
              </div>
            )
          ) : null}
        </div>
      </div>
    </GlassCard>
  );
}
