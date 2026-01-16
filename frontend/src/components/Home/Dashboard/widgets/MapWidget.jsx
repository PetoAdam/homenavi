import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { faMap } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import useDeviceHubDevices from '../../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../../hooks/useErsInventory';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import { mergeDevicePlacementsFromErs, mergeRoomsFromErs } from '../../../Map/mapHydrate';
import { roomPolygonToPath } from '../../../Map/mapGeometry';
import { readFavoriteFieldsFromErsMeta } from '../../../Map/mapErsMeta';
import { pickStateValue } from '../../../Map/mapDeviceUtils';
import { formatMetricValueAndUnitForKey } from '../../../../utils/stateFormat';
import './MapWidget.css';

function computeRoomCenter(points) {
  if (!Array.isArray(points) || points.length === 0) return null;
  let sumX = 0;
  let sumY = 0;
  let count = 0;
  points.forEach((p) => {
    if (Number.isFinite(p?.x) && Number.isFinite(p?.y)) {
      sumX += p.x;
      sumY += p.y;
      count++;
    }
  });
  if (count === 0) return null;
  return { x: sumX / count, y: sumY / count };
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

function parseAspectRatio(raw) {
  if (!raw || typeof raw !== 'string') return null;
  const trimmed = raw.trim();
  if (!trimmed || trimmed === 'auto') return null;
  if (trimmed.includes('/')) {
    const [aRaw, bRaw] = trimmed.split('/');
    const a = Number.parseFloat(String(aRaw).trim());
    const b = Number.parseFloat(String(bRaw).trim());
    if (Number.isFinite(a) && Number.isFinite(b) && a > 0 && b > 0) return a / b;
    return null;
  }
  const n = Number.parseFloat(trimmed);
  if (Number.isFinite(n) && n > 0) return n;
  return null;
}

export default function MapWidget({
  instanceId,
  settings = {},
  // enabled is unused
  editMode,
  onSettings,
  onRemove,
}) {
  const navigate = useNavigate();
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  // Track if user is dragging to prevent navigation (must be before conditional returns)
  const [isDragging, setIsDragging] = useState(false);
  const dragStartPos = useRef({ x: 0, y: 0 });

  const previewRef = useRef(null);
  const contentRef = useRef(null);

  const { devices: realtimeDevices, loading: realtimeLoading } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
  });

  const { devices, rooms, loading } = useErsInventory({
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

  const bounds = useMemo(() => computeBounds({ rooms: layoutRooms, placements }), [layoutRooms, placements]);

  const aspect = useMemo(() => {
    if (!bounds) return null;
    const w = bounds.maxX - bounds.minX;
    const h = bounds.maxY - bounds.minY;
    if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) return null;
    return `${w} / ${h}`;
  }, [bounds]);

  useEffect(() => {
    const previewEl = previewRef.current;
    const contentEl = contentRef.current;
    if (!previewEl || !contentEl) return undefined;

    let rafId = 0;
    let lastEmitted = 0;

    const computeAndEmit = () => {
      // Goal: grow the react-grid-layout item so the map never clips.
      // Use width + aspect ratio to get a predictable height that doesn't depend on
      // the currently clipped rendered size.

      const shellEl = contentEl.closest('.widget-shell');
      const headerEl = shellEl?.querySelector?.('.widget-shell__header');
      const headerHeight = headerEl ? headerEl.getBoundingClientRect().height : 0;

      const shellStyles = shellEl ? window.getComputedStyle(shellEl) : null;
      const paddingTop = shellStyles ? parseFloat(shellStyles.paddingTop) || 0 : 0;
      const paddingBottom = shellStyles ? parseFloat(shellStyles.paddingBottom) || 0 : 0;

      const previewWidth = previewEl.getBoundingClientRect().width;
      if (!Number.isFinite(previewWidth) || previewWidth <= 0) return;

      const ratio = parseAspectRatio(settings?.aspectRatio) || parseAspectRatio(aspect) || 16 / 10;
      const mapHeight = previewWidth / ratio;
      if (!Number.isFinite(mapHeight) || mapHeight <= 0) return;

      // Fixed overhead for wrapper/border/gap and rounding.
      const desiredTotalHeight = paddingTop + paddingBottom + headerHeight + mapHeight + 20;

      const next = Math.round(desiredTotalHeight);
      if (Math.abs(next - (lastEmitted || 0)) < 8) return;
      lastEmitted = next;

      window.dispatchEvent(new CustomEvent('homenavi:widgetDesiredHeight', {
        detail: { instanceId, heightPx: next },
      }));
    };

    const schedule = () => {
      if (rafId) return;
      rafId = window.requestAnimationFrame(() => {
        rafId = 0;
        computeAndEmit();
      });
    };

    // Delay initial calculation to allow aspect-ratio/layout to settle
    const timeoutId = setTimeout(schedule, 150);

    let ro;
    try {
      ro = new ResizeObserver(() => schedule());
      ro.observe(previewEl);
      if (contentEl) ro.observe(contentEl);
      const shellEl = contentEl.closest('.widget-shell');
      if (shellEl) ro.observe(shellEl);
    } catch {
      // ignore
    }

    return () => {
      clearTimeout(timeoutId);
      if (rafId) {
        try {
          window.cancelAnimationFrame(rafId);
        } catch {
          // ignore
        }
        rafId = 0;
      }
      try {
        ro?.disconnect();
      } catch {
        // ignore
      }
    };
  }, [aspect, instanceId, settings?.aspectRatio]);

  // Build a map of placement key -> device info for navigation
  const placementDeviceMap = useMemo(() => {
    const map = new Map();
    Object.entries(placements).forEach(([key]) => {
      const device = (devices || []).find((d) => 
        d.hdpId === key || d.id === key || d.ersId === key
      );
      if (device) {
        map.set(key, device);
      }
    });
    return map;
  }, [placements, devices]);

  const handleOpenMap = useCallback(() => {
    if (editMode || isDragging) return;
    navigate('/map');
  }, [editMode, isDragging, navigate]);

  // Drag detection handlers
  const handlePointerDown = useCallback((e) => {
    dragStartPos.current = { x: e.clientX, y: e.clientY };
    setIsDragging(false);
  }, []);

  const handlePointerMove = useCallback((e) => {
    const dx = Math.abs(e.clientX - dragStartPos.current.x);
    const dy = Math.abs(e.clientY - dragStartPos.current.y);
    if (dx > 5 || dy > 5) {
      setIsDragging(true);
    }
  }, []);

  const handleWidgetClick = useCallback(() => {
    // Don't navigate if we were dragging or in edit mode
    if (isDragging || editMode) return;
    handleOpenMap();
  }, [isDragging, editMode, handleOpenMap]);

  // Handle room click - navigate to map focused on that room
  const handleRoomClick = useCallback((e, roomId) => {
    if (editMode) return;
    e.stopPropagation();
    navigate(`/map?room=${encodeURIComponent(roomId)}`);
  }, [editMode, navigate]);

  // Handle device click - navigate to device detail
  const handleDeviceClick = useCallback((e, deviceKey) => {
    if (editMode) return;
    e.stopPropagation();
    const device = placementDeviceMap.get(deviceKey);
    if (device) {
      const id = device.hdpId || device.id || device.ersId;
      navigate(`/devices/${encodeURIComponent(id)}`);
    }
  }, [editMode, placementDeviceMap, navigate]);

  const previewLoading = Boolean(bootstrapping || loading || realtimeLoading);

  // Determine status for unauthorized
  let status = null;
  if (!isResidentOrAdmin) {
    status = 401;
  }

  return (
    <WidgetShell
      title={settings.title || 'Map'}
      icon={faMap}
      subtitle={`${layoutRooms.length} rooms · ${Object.keys(placements).length} devices`}
      className="map-widget"
      loading={previewLoading && isResidentOrAdmin}
      status={status}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      interactive={!editMode}
      onClick={handleWidgetClick}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      role={!editMode ? 'button' : undefined}
      tabIndex={!editMode ? 0 : undefined}
      onKeyDown={(e) => {
        if (editMode) return;
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleOpenMap();
        }
      }}
    >
      <div className="map-widget__content" ref={contentRef}>
        {/* Map preview SVG */}
        <div
          className="map-widget__preview"
          aria-label="Apartment map preview"
          style={aspect ? { aspectRatio: aspect } : undefined}
          ref={previewRef}
        >
          {isResidentOrAdmin && !previewLoading && bounds && (
            <svg
              className="map-widget__svg"
              viewBox={`${bounds.minX} ${bounds.minY} ${bounds.maxX - bounds.minX} ${bounds.maxY - bounds.minY}`}
              preserveAspectRatio="xMidYMid meet"
            >
              {/* Rooms */}
              {layoutRooms.map((room) => {
                const d = roomPolygonToPath(room.points || []);
                if (!d) return null;
                const center = computeRoomCenter(room.points);
                return (
                  <g key={room.id}>
                    <path
                      d={d}
                      className="map-widget__room"
                      fill="var(--color-glass-bg-light)"
                      stroke="var(--color-glass-border)"
                      strokeWidth="2"
                      onClick={(e) => handleRoomClick(e, room.id)}
                    />
                    {center && room.name && (
                      <text
                        x={center.x}
                        y={center.y}
                        className="map-widget__room-label"
                      >
                        {room.name}
                      </text>
                    )}
                  </g>
                );
              })}

              {/* Device placements */}
              {Object.entries(placements).map(([key, pl]) => {
                const x = Number(pl?.x);
                const y = Number(pl?.y);
                if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
                const device = placementDeviceMap.get(key);
                const isOnline = device?.online !== false;
                const deviceName = device?.displayName || device?.name || '';
                const deviceState = device?.state?.on !== undefined 
                  ? (device.state.on ? 'On' : 'Off')
                  : (isOnline ? '' : 'Offline');

        const stateObj = device?.state && typeof device.state === 'object' ? device.state : null;
        const favorites = device ? readFavoriteFieldsFromErsMeta(device) : [];
        const allCaps = [
          ...(Array.isArray(device?.capabilities) ? device.capabilities : []),
          ...(Array.isArray(device?.state?.capabilities) ? device.state.capabilities : []),
        ];
        const favoriteLines = favorites
          .map((favoriteKey) => {
            const raw = favoriteKey && stateObj ? pickStateValue(stateObj, favoriteKey) : undefined;
            const { valueText, unit } = formatMetricValueAndUnitForKey(favoriteKey, raw, allCaps);
            const text = valueText ? `${valueText}${unit || ''}` : '';
            return text ? { key: favoriteKey, text } : null;
          })
          .filter(Boolean)
          .slice(0, 2);
                return (
                  <g key={key}>
                    <circle
                      cx={x}
                      cy={y}
                      r="8"
                      className="map-widget__device"
                      fill={isOnline ? 'var(--color-primary)' : 'var(--color-secondary-light)'}
                      opacity="0.85"
                      onClick={(e) => handleDeviceClick(e, key)}
                    />
                    {deviceName && (
                      <text
                        x={x}
                        y={y + 18}
                        className="map-widget__device-label"
                      >
                        <tspan x={x}>
                          {deviceName}
                          {deviceState ? ` · ${deviceState}` : ''}
                        </tspan>
                        {favoriteLines.map((line, idx) => (
                          <tspan
                            key={`${key}-fav-${line.key}`}
                            x={x}
                            dy={idx === 0 ? 10 : 10}
                            className="map-widget__device-favorite"
                          >
                            {line.text}
                          </tspan>
                        ))}
                      </text>
                    )}
                  </g>
                );
              })}
            </svg>
          )}

          {isResidentOrAdmin && !previewLoading && !bounds && (
            <div className="map-widget__empty">
              <FontAwesomeIcon icon={faMap} className="map-widget__empty-icon" />
              <span>No map data</span>
            </div>
          )}
        </div>
      </div>
    </WidgetShell>
  );
}

MapWidget.defaultHeight = 5;
