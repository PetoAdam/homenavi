import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { faChartLine } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassPill from '../common/GlassPill/GlassPill';
import PageHeader from '../common/PageHeader/PageHeader';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import { useAuth } from '../../context/AuthContext';
import DeviceTile from './DeviceTile';
import { deleteDevice, renameDevice, sendDeviceCommand, setDeviceIcon } from '../../services/deviceHubService';
import { listStatePoints } from '../../services/historyService';
import HistoryChart from '../History/HistoryChart';
import './DeviceDetail.css';

function toRFC3339(value) {
  if (!value) return '';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '';
  return d.toISOString();
}

function safeDecode(value) {
  if (!value) return '';
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function toDatetimeLocalValue(date) {
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) return '';
  const pad = n => String(n).padStart(2, '0');
  const y = date.getFullYear();
  const m = pad(date.getMonth() + 1);
  const d = pad(date.getDate());
  const hh = pad(date.getHours());
  const mm = pad(date.getMinutes());
  return `${y}-${m}-${d}T${hh}:${mm}`;
}

function todayLocalDateValue() {
  const now = new Date();
  const pad = n => String(n).padStart(2, '0');
  const y = now.getFullYear();
  const m = pad(now.getMonth() + 1);
  const d = pad(now.getDate());
  return `${y}-${m}-${d}`;
}

function pad2(n) {
  return String(n).padStart(2, '0');
}

function time24ToHM(time) {
  const t = typeof time === 'string' ? time : '';
  const m = t.match(/^(\d{2}):(\d{2})$/);
  if (!m) return { hour: '00', minute: '00' };
  const hh = Math.min(23, Math.max(0, Number(m[1])));
  const mm = Math.min(59, Math.max(0, Number(m[2])));
  return { hour: pad2(hh), minute: pad2(mm) };
}

function hmToTime24(hour, minute) {
  const hh = Math.min(23, Math.max(0, Number(hour) || 0));
  const mm = Math.min(59, Math.max(0, Number(minute) || 0));
  return `${pad2(hh)}:${pad2(mm)}`;
}

function wrapInt(value, maxInclusive) {
  const max = Number(maxInclusive);
  if (!Number.isFinite(max) || max <= 0) return 0;
  const n = Number(value);
  if (!Number.isFinite(n)) return 0;
  const mod = ((n % (max + 1)) + (max + 1)) % (max + 1);
  return mod;
}

function splitDatetimeLocal(value) {
  const v = typeof value === 'string' ? value : '';
  const idx = v.indexOf('T');
  if (idx === -1) return { date: v || '', time: '' };
  return {
    date: v.slice(0, idx),
    time: v.slice(idx + 1, idx + 6),
  };
}

function TimePartsSelect({ value, onChange, ariaLabelPrefix }) {
  const parts = time24ToHM(value);
  const hourN = wrapInt(parts.hour, 23);
  const minuteN = wrapInt(parts.minute, 59);

  const setHour = (nextHour) => {
    const h = wrapInt(nextHour, 23);
    onChange(hmToTime24(h, minuteN));
  };

  const setMinute = (nextMinute) => {
    const m = wrapInt(nextMinute, 59);
    onChange(hmToTime24(hourN, m));
  };

  return (
    <div className="device-history-time-stepper" aria-label={ariaLabelPrefix}>
      <div className="device-history-time-input-wrap">
        <input
          type="number"
          inputMode="numeric"
          min={0}
          max={23}
          value={pad2(hourN)}
          onChange={e => setHour(e.target.value)}
          aria-label={`${ariaLabelPrefix} hour`}
        />
        <div className="device-history-stepper" aria-hidden="false">
          <button
            type="button"
            className="device-history-stepper-btn"
            aria-label="Increase hour"
            onClick={() => setHour(hourN + 1)}
          >
            ▲
          </button>
          <button
            type="button"
            className="device-history-stepper-btn"
            aria-label="Decrease hour"
            onClick={() => setHour(hourN - 1)}
          >
            ▼
          </button>
        </div>
      </div>

      <span className="device-history-time-colon" aria-hidden="true">:</span>

      <div className="device-history-time-input-wrap">
        <input
          type="number"
          inputMode="numeric"
          min={0}
          max={59}
          value={pad2(minuteN)}
          onChange={e => setMinute(e.target.value)}
          aria-label={`${ariaLabelPrefix} minute`}
        />
        <div className="device-history-stepper" aria-hidden="false">
          <button
            type="button"
            className="device-history-stepper-btn"
            aria-label="Increase minute"
            onClick={() => setMinute(minuteN + 1)}
          >
            ▲
          </button>
          <button
            type="button"
            className="device-history-stepper-btn"
            aria-label="Decrease minute"
            onClick={() => setMinute(minuteN - 1)}
          >
            ▼
          </button>
        </div>
      </div>
    </div>
  );
}

function parseBooleanish(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value !== 'string') return null;
  const v = value.trim().toLowerCase();
  if (['true', 'on', '1', 'yes', 'enabled', 'active', 'detected', 'present', 'open'].includes(v)) return true;
  if (['false', 'off', '0', 'no', 'disabled', 'inactive', 'clear', 'absent', 'closed'].includes(v)) return false;
  return null;
}

function parseNumberish(value) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value !== 'string') return null;
  const trimmed = value.trim();
  if (!trimmed) return null;
  const n = Number(trimmed);
  return Number.isFinite(n) ? n : null;
}

function extractMetricSeries(points) {
  const metricMap = new Map();
  const reserved = new Set([
    'schema', 'device_id', 'deviceid', 'external_id', 'externalid', 'protocol', 'topic', 'retained',
    'ts', 'timestamp', 'time', 'received_at', 'receivedat',
  ]);

  const binaryKeys = new Set([
    'state', 'on', 'power',
    'contact', 'open', 'closed',
    'occupancy', 'motion', 'presence',
    'water_leak', 'leak', 'moisture',
    'smoke', 'tamper',
    'battery_low', 'low_battery',
  ]);

  (Array.isArray(points) ? points : []).forEach(p => {
    const payload = p?.payload;
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return;

    const rawState = payload.state;
    const state = rawState && typeof rawState === 'object' && !Array.isArray(rawState)
      ? rawState
      : payload;

    Object.entries(state).forEach(([key, raw]) => {
      if (!key) return;
      const keyLower = key.toLowerCase();
      if (reserved.has(keyLower)) return;

      let kind = null;
      let value = null;

      // Prefer numeric for numeric-like values. Only coerce 0/1 into boolean for known binary keys.
      const num = parseNumberish(raw);
      if (num !== null) {
        if (binaryKeys.has(keyLower) && (num === 0 || num === 1)) {
          kind = 'boolean';
          value = num === 1;
        } else {
          kind = 'number';
          value = num;
        }
      } else {
        const bool = parseBooleanish(raw);
        if (bool !== null) {
          kind = 'boolean';
          value = bool;
        }
      }

      if (!kind) return;
      if (!p?.ts) return;

      const existing = metricMap.get(key) || { key, kind, series: [] };
      // if we see mixed types, keep boolean if any values are booleanish, otherwise number
      if (existing.kind !== kind) {
        existing.kind = existing.kind === 'boolean' || kind === 'boolean' ? 'boolean' : 'number';
      }
      existing.series.push({ ts: p.ts, value });
      metricMap.set(key, existing);
    });
  });

  return Array.from(metricMap.values())
    .sort((a, b) => a.key.localeCompare(b.key, undefined, { sensitivity: 'base' }));
}

export default function DeviceDetail() {
  const navigate = useNavigate();
  const params = useParams();
  const encodedId = params.deviceId || '';
  const deviceId = useMemo(() => safeDecode(encodedId), [encodedId]);

  const { user, accessToken } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const { devices, loading, error } = useDeviceHubDevices({ enabled: Boolean(isResidentOrAdmin), metadataMode: 'rest' });

  const device = useMemo(() => {
    if (!devices?.length) return null;
    return devices.find(d => d.id === deviceId) || null;
  }, [devices, deviceId]);

  const [pendingCommand, setPendingCommand] = useState(false);

  const handleCommand = useCallback(async (dev, payload) => {
    if (!dev?.id) return;
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    setPendingCommand(true);
    try {
      const res = await sendDeviceCommand(dev.id, payload, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to send command');
      return res.data;
    } finally {
      setPendingCommand(false);
    }
  }, [accessToken]);

  const handleRename = useCallback(async (dev, name) => {
    if (!dev?.id) return;
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const trimmed = typeof name === 'string' ? name.trim() : '';
    const res = await renameDevice(dev.id, trimmed, accessToken);
    if (!res.success) throw new Error(res.error || 'Unable to rename device');
    return res.data;
  }, [accessToken]);

  const handleUpdateIcon = useCallback(async (dev, iconKey) => {
    if (!dev?.id) return;
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await setDeviceIcon(dev.id, iconKey, accessToken);
    if (!res.success) throw new Error(res.error || 'Unable to update icon');
    return res.data;
  }, [accessToken]);

  const handleDelete = useCallback(async (dev, options = {}) => {
    if (!dev?.id) return;
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await deleteDevice(dev.id, accessToken, options);
    if (!res.success) throw new Error(res.error || 'Unable to delete device');
    navigate('/devices');
  }, [accessToken, navigate]);

  const [rangePreset, setRangePreset] = useState('24h');
  const [fromLocal, setFromLocal] = useState('');
  const [toLocal, setToLocal] = useState('');
  const [limitEnabled, setLimitEnabled] = useState(false);
  const [limit, setLimit] = useState(300);
  const [order, setOrder] = useState('desc');

  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState(null);
  const [historyPoints, setHistoryPoints] = useState([]);

  useEffect(() => {
    // Default to "last 24 hours", and keep it visible.
    const now = new Date();
    const from = new Date(now.getTime() - (24 * 60 * 60 * 1000));
    setFromLocal(prev => prev || toDatetimeLocalValue(from));
    setToLocal(prev => prev || toDatetimeLocalValue(now));
    setRangePreset(prev => prev || '24h');
  }, []);

  useEffect(() => {
    if (rangePreset === 'custom') return;
    const now = new Date();
    let ms = 24 * 60 * 60 * 1000;
    if (rangePreset === '1h') ms = 1 * 60 * 60 * 1000;
    if (rangePreset === '6h') ms = 6 * 60 * 60 * 1000;
    if (rangePreset === '7d') ms = 7 * 24 * 60 * 60 * 1000;
    const from = new Date(now.getTime() - ms);
    setFromLocal(toDatetimeLocalValue(from));
    setToLocal(toDatetimeLocalValue(now));
  }, [rangePreset]);

  const fetchHistory = useCallback(async () => {
    if (!deviceId) return;
    if (!accessToken) {
      setHistoryError('Authentication required');
      return;
    }

    setHistoryLoading(true);
    setHistoryError(null);
    try {
      const res = await listStatePoints(deviceId, {
        from: toRFC3339(fromLocal),
        to: toRFC3339(toLocal),
        limit: limitEnabled ? limit : undefined,
        order,
      }, accessToken);

      if (!res.success) {
        throw new Error(res.error || 'Unable to load history');
      }

      const points = Array.isArray(res.data?.points) ? res.data.points : [];
      const normalized = points.map(p => {
        let payload = p?.payload;
        if (typeof payload === 'string') {
          try {
            payload = JSON.parse(payload);
          } catch {
            payload = {};
          }
        }
        return {
          ts: p?.ts,
          payload: payload && typeof payload === 'object' ? payload : {},
          retained: Boolean(p?.retained),
          topic: p?.topic || '',
        };
      });
      setHistoryPoints(normalized);
    } catch (err) {
      setHistoryError(err?.message || 'Unable to load history');
      setHistoryPoints([]);
    } finally {
      setHistoryLoading(false);
    }
  }, [accessToken, deviceId, fromLocal, toLocal, limitEnabled, limit, order]);

  const autoFetchedRef = useRef(false);

  const canQueryHistory = Boolean(isResidentOrAdmin && accessToken && deviceId);

  useEffect(() => {
    if (!canQueryHistory) return;
    if (autoFetchedRef.current) return;
    // Auto-run once so the page doesn't look empty with the default "Last 24 hours" range.
    autoFetchedRef.current = true;
    fetchHistory();
  }, [canQueryHistory, fetchHistory]);

  const metrics = useMemo(() => {
    const raw = extractMetricSeries(historyPoints);
    if (raw.length === 0) return [];
    if (order === 'desc') {
      return raw.map(m => ({
        ...m,
        series: Array.isArray(m.series) ? m.series.slice().reverse() : [],
      }));
    }
    return raw;
  }, [historyPoints, order]);

  const unitForMetric = useCallback((key) => {
    const k = (key || '').toString().toLowerCase();
    const fallback = {
      temperature: '°C',
      humidity: '%',
      battery: '%',
      voltage: 'mV',
      linkquality: 'lqi',
    };

    const caps = Array.isArray(device?.capabilities) ? device.capabilities : [];
    for (const cap of caps) {
      if (!cap || typeof cap !== 'object') continue;
      const id = (cap.id || cap.property || cap.name || '').toString().toLowerCase();
      if (id && id === k) {
        const unit = cap.unit || cap.units || '';
        return unit || (fallback[k] || '');
      }
    }
    return fallback[k] || '';
  }, [device?.capabilities]);

  const originRefs = useRef(new Map());
  const overlayCardRef = useRef(null);
  const [overlay, setOverlay] = useState(null); // { key, fromRect }
  const [overlayPhase, setOverlayPhase] = useState(''); // opening | open | closing
  const prefersReducedMotion = useMemo(() => {
    try {
      return Boolean(window?.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches);
    } catch {
      return false;
    }
  }, []);

  const isMobile = useMemo(() => {
    try {
      return Boolean(window?.matchMedia?.('(max-width: 640px)')?.matches);
    } catch {
      return false;
    }
  }, []);

  const overlayMetric = useMemo(() => {
    if (!overlay?.key) return null;
    return metrics.find(m => m.key === overlay.key) || null;
  }, [metrics, overlay?.key]);

  const openOverlay = useCallback((key, el) => {
    if (!key) return;
    if (overlay) return;
    const node = el || originRefs.current.get(key);
    if (!node?.getBoundingClientRect) return;
    const fromRect = node.getBoundingClientRect();
    setOverlay({ key, fromRect });
    setOverlayPhase('opening');
  }, [overlay]);

  const closeOverlay = useCallback(() => {
    if (!overlay) return;
    setOverlayPhase('closing');
  }, [overlay]);

  useLayoutEffect(() => {
    if (!overlay || !overlayCardRef.current) return;

    if (prefersReducedMotion) {
      if (overlayPhase === 'opening') setOverlayPhase('open');
      if (overlayPhase === 'closing') {
        setOverlay(null);
        setOverlayPhase('');
      }
      return;
    }

    const el = overlayCardRef.current;
    const toRect = el.getBoundingClientRect();
    if (!toRect.width || !toRect.height) return;

    const animateFromRect = (fromRect, { reverse } = {}) => {
      if (!fromRect?.width || !fromRect?.height) return null;
      const dx = fromRect.left - toRect.left;
      const dy = fromRect.top - toRect.top;
      const sx = fromRect.width / toRect.width;
      const sy = fromRect.height / toRect.height;
      const from = { transformOrigin: 'top left', transform: `translate(${dx}px, ${dy}px) scale(${sx}, ${sy})` };
      const to = { transformOrigin: 'top left', transform: 'translate(0px, 0px) scale(1, 1)' };
      return el.animate(reverse ? [to, from] : [from, to], {
        duration: 320,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
        fill: 'both',
      });
    };

    if (overlayPhase === 'opening') {
      const anim = animateFromRect(overlay.fromRect);
      if (!anim) return;
      anim.onfinish = () => setOverlayPhase('open');
      return () => anim.cancel();
    }

    if (overlayPhase === 'closing') {
      const originEl = originRefs.current.get(overlay.key);
      const originRect = originEl?.getBoundingClientRect?.();
      if (!originRect) {
        setOverlay(null);
        setOverlayPhase('');
        return;
      }
      const anim = animateFromRect(originRect, { reverse: true });
      if (!anim) {
        setOverlay(null);
        setOverlayPhase('');
        return;
      }
      anim.onfinish = () => {
        setOverlay(null);
        setOverlayPhase('');
      };
      return () => anim.cancel();
    }
  }, [overlay, overlayPhase, prefersReducedMotion]);

  // Close on Escape and manage body scroll & focus when overlay is open
  useEffect(() => {
    if (!overlay) return undefined;
    const onKey = (e) => {
      if (e.key === 'Escape') closeOverlay();
    };
    document.addEventListener('keydown', onKey);
    // prevent body scroll
    document.body.classList.add('overlay-open');
    // focus overlay card when opened
    const focusTimeout = setTimeout(() => {
      try { overlayCardRef.current?.focus?.(); } catch (err) {}
    }, 40);
    return () => {
      clearTimeout(focusTimeout);
      document.removeEventListener('keydown', onKey);
      document.body.classList.remove('overlay-open');
    };
  }, [overlay, closeOverlay]);

  if (!isResidentOrAdmin) {
    return (
      <UnauthorizedView
        title="Device"
        message="You do not have permission to view this page."
      />
    );
  }

  return (
    <div className="device-detail-page">
      <PageHeader
        title="Device"
        subtitle={deviceId}
        showBack
        onBack={() => navigate(-1)}
        className="device-detail-header"
      />

      {error ? (
        <GlassCard className="device-detail-error" interactive={false}>
          <div className="device-detail-error-text">{error}</div>
        </GlassCard>
      ) : null}

      {loading && !device ? (
        <GlassCard className="device-detail-loading" interactive={false}>
          <div className="device-detail-loading-text">Loading device…</div>
        </GlassCard>
      ) : null}

      {!loading && !device ? (
        <GlassCard className="device-detail-missing" interactive={false}>
          <div className="device-detail-missing-text">Device not found.</div>
        </GlassCard>
      ) : null}

      <div className="device-detail-grid">
        <div className="device-detail-grid-left">
          {device ? (
            <DeviceTile
              device={device}
              pending={pendingCommand}
              onCommand={handleCommand}
              onRename={handleRename}
              onUpdateIcon={handleUpdateIcon}
              onDelete={handleDelete}
              actionLayout="buttons"
            />
          ) : null}
        </div>

        <div className="device-detail-grid-right">
          <GlassCard className="device-history-controls-card" interactive={false}>
            <div className="device-history-header">
              <div className="device-history-title">
                <span className="device-history-icon">
                  <span aria-hidden="true">▦</span>
                </span>
                <span>History</span>
              </div>
              <span className="device-history-hint">Default: Last 24 hours</span>
            </div>

            <div className="device-history-controls">
              <label className="device-history-field device-history-field--range">
                <span>Range</span>
                <select value={rangePreset} onChange={e => setRangePreset(e.target.value)}>
                  <option value="1h">Last 1 hour</option>
                  <option value="6h">Last 6 hours</option>
                  <option value="24h">Last 24 hours</option>
                  <option value="7d">Last 7 days</option>
                  <option value="custom">Custom</option>
                </select>
              </label>
              <label className="device-history-field device-history-field--from">
                <span>From</span>
                <div className="device-history-datetime-split">
                  <input
                    type="date"
                    className="device-history-date"
                    value={splitDatetimeLocal(fromLocal).date}
                    onChange={e => {
                      const nextDate = e.target.value;
                      const prev = splitDatetimeLocal(fromLocal);
                      const nextTime = prev.time || '00:00';
                      setFromLocal(nextDate ? `${nextDate}T${nextTime}` : '');
                      if (rangePreset !== 'custom') setRangePreset('custom');
                    }}
                    aria-label="From date"
                  />
                  <TimePartsSelect
                    value={splitDatetimeLocal(fromLocal).time || '00:00'}
                    ariaLabelPrefix="From time"
                    onChange={(nextTime) => {
                      const prev = splitDatetimeLocal(fromLocal);
                      const nextDate = prev.date || todayLocalDateValue();
                      setFromLocal(`${nextDate}T${nextTime}`);
                      if (rangePreset !== 'custom') setRangePreset('custom');
                    }}
                  />
                </div>
              </label>
              <label className="device-history-field device-history-field--to">
                <span>To</span>
                <div className="device-history-datetime-split">
                  <input
                    type="date"
                    className="device-history-date"
                    value={splitDatetimeLocal(toLocal).date}
                    onChange={e => {
                      const nextDate = e.target.value;
                      const prev = splitDatetimeLocal(toLocal);
                      const nextTime = prev.time || '00:00';
                      setToLocal(nextDate ? `${nextDate}T${nextTime}` : '');
                      if (rangePreset !== 'custom') setRangePreset('custom');
                    }}
                    aria-label="To date"
                  />
                  <TimePartsSelect
                    value={splitDatetimeLocal(toLocal).time || '00:00'}
                    ariaLabelPrefix="To time"
                    onChange={(nextTime) => {
                      const prev = splitDatetimeLocal(toLocal);
                      const nextDate = prev.date || todayLocalDateValue();
                      setToLocal(`${nextDate}T${nextTime}`);
                      if (rangePreset !== 'custom') setRangePreset('custom');
                    }}
                  />
                </div>
              </label>
              <label className="device-history-field device-history-field--limit">
                <span>Limit</span>
                <div className={`device-history-limit-inline${limitEnabled ? ' on' : ' off'}`}
                >
                  <button
                    type="button"
                    className={`device-history-limit-chip${limitEnabled ? ' on' : ' off'}`}
                    aria-pressed={limitEnabled ? 'true' : 'false'}
                    aria-label={limitEnabled ? 'Disable limit' : 'Enable limit'}
                    title={limitEnabled ? 'Limit enabled' : 'Limit disabled'}
                    onClick={() => setLimitEnabled(v => !v)}
                  >
                    <span className="device-history-limit-check" aria-hidden="true">✓</span>
                    {limitEnabled ? <span>Limit</span> : null}
                  </button>

                  {limitEnabled ? (
                    <div className="device-history-limit-input-wrap">
                      <input
                        type="number"
                        min={1}
                        max={5000}
                        step={10}
                        value={limit}
                        onChange={e => setLimit(Number(e.target.value) || 1)}
                        aria-label="Limit (points)"
                      />
                      <div className="device-history-stepper" aria-hidden="false">
                        <button
                          type="button"
                          className="device-history-stepper-btn"
                          aria-label="Increase limit"
                          onClick={() => setLimit(v => Math.min(5000, (Number(v) || 1) + 10))}
                        >
                          ▲
                        </button>
                        <button
                          type="button"
                          className="device-history-stepper-btn"
                          aria-label="Decrease limit"
                          onClick={() => setLimit(v => Math.max(1, (Number(v) || 1) - 10))}
                        >
                          ▼
                        </button>
                      </div>
                    </div>
                  ) : null}
                </div>
              </label>
              <label className="device-history-field device-history-field--order">
                <span>Order</span>
                <select value={order} onChange={e => setOrder(e.target.value)}>
                  <option value="desc">Newest first</option>
                  <option value="asc">Oldest first</option>
                </select>
              </label>
            </div>

            {historyError ? <div className="device-history-error">{historyError}</div> : null}
            <div className="device-history-footer">
              <span>{historyPoints.length} points loaded</span>
              <GlassPill
                icon={faChartLine}
                text={historyLoading ? 'Loading…' : 'Run query'}
                tone={canQueryHistory ? 'success' : 'warning'}
                onClick={canQueryHistory && !historyLoading ? fetchHistory : undefined}
                title={canQueryHistory ? 'Fetch history points' : 'Sign in as a resident to query history'}
                className="device-history-query-pill"
              />
            </div>
          </GlassCard>

          {metrics.length === 0 ? (
            <GlassCard className="device-history-empty" interactive={false}>
              <div className="device-history-empty-text">
                No metrics found in history payloads yet.
              </div>
            </GlassCard>
          ) : (
            <div className="device-history-charts-area">
              <section className="device-history-grid">
                {metrics.map(metric => {
                  const unit = unitForMetric(metric.key);
                  const isOriginHidden = overlay?.key === metric.key;
                  const isOpen = overlay?.key === metric.key && overlayPhase === 'open';
                  return (
                    <GlassCard
                      key={metric.key}
                      ref={(el) => {
                        if (el) originRefs.current.set(metric.key, el);
                        else originRefs.current.delete(metric.key);
                      }}
                      className={`device-history-metric-card${isOriginHidden ? ' origin-hidden' : ''}`}
                      interactive={false}
                      onClick={(e) => {
                        if (overlay) {
                          if (isOpen) closeOverlay();
                          return;
                        }
                        openOverlay(metric.key, e.currentTarget);
                      }}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          if (overlay) {
                            if (isOpen) closeOverlay();
                            return;
                          }
                          openOverlay(metric.key, originRefs.current.get(metric.key));
                        }
                      }}
                      aria-expanded={isOpen ? 'true' : 'false'}
                    >
                      <HistoryChart
                        title={metric.key}
                        series={metric.series}
                        unit={unit}
                        height={180}
                      />
                    </GlassCard>
                  );
                })}
              </section>

              {overlay && overlayMetric ? (
                <div className="device-history-overlay" aria-hidden="false">
                  <div className="device-history-overlay-backdrop" onClick={() => { if (overlayPhase === 'open') closeOverlay(); }} />

                  <GlassCard
                    ref={overlayCardRef}
                    className="device-history-overlay-card"
                    interactive={false}
                    role="button"
                    tabIndex={0}
                    onClick={() => {
                      if (overlayPhase === 'open') closeOverlay();
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        if (overlayPhase === 'open') closeOverlay();
                      }
                    }}
                  >
                    <button
                      type="button"
                      className="device-history-overlay-close"
                      aria-label="Close"
                      onClick={(e) => { e.stopPropagation(); if (overlayPhase === 'open') closeOverlay(); }}
                    >
                      ×
                    </button>
                    <HistoryChart
                      title={overlayMetric.key}
                      series={overlayMetric.series}
                      unit={unitForMetric(overlayMetric.key)}
                      height={isMobile ? 300 : 420}
                    />
                    <div className="device-history-expanded-hint">
                      Click again to return to the grid.
                    </div>
                  </GlassCard>
                </div>
              ) : null}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
