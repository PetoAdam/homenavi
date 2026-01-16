import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChartLine, faQuestionCircle } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import useDeviceHubDevices from '../../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../../hooks/useErsInventory';
import { listStatePoints } from '../../../../services/historyService';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import HistoryChart from '../../../History/HistoryChart';
import './DeviceGraphWidget.css';

function toRFC3339(dt) {
  if (!dt) return '';
  try {
    return new Date(dt).toISOString();
  } catch {
    return '';
  }
}

function resolveMetricValue(state, metricKey) {
  if (!state || typeof state !== 'object' || Array.isArray(state)) return undefined;
  const key = (metricKey || '').toString().trim();
  if (!key) return undefined;

  if (state[key] !== undefined) return state[key];

  const lower = key.toLowerCase();
  for (const [k, v] of Object.entries(state)) {
    if (!k) continue;
    if (k.toString().toLowerCase() === lower) return v;
  }

  const camel = lower.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
  if (state[camel] !== undefined) return state[camel];

  return undefined;
}

function defaultRangePreset(preset) {
  const v = (preset || '').toString().toLowerCase();
  if (['1h', '6h', '24h', '7d'].includes(v)) return v;
  return '24h';
}

function rangeToMs(preset) {
  if (preset === '1h') return 1 * 60 * 60 * 1000;
  if (preset === '6h') return 6 * 60 * 60 * 1000;
  if (preset === '7d') return 7 * 24 * 60 * 60 * 1000;
  return 24 * 60 * 60 * 1000;
}

export default function DeviceGraphWidget({
  settings = {},
  editMode,
  onSettings,
  onRemove,
}) {
  const { user, accessToken } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const deviceId = settings.device_id || settings.hdp_device_id || settings.ers_device_id || '';
  const metricKey = settings.metric_key || settings.metric || settings.field || '';
  const preset = defaultRangePreset(settings.range_preset);

  const { devices: realtimeDevices } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'rest',
  });

  const { devices: ersDevices } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const device = useMemo(() => {
    if (!deviceId) return null;
    const ers = Array.isArray(ersDevices) ? ersDevices : [];
    const rt = Array.isArray(realtimeDevices) ? realtimeDevices : [];
    return (
      ers.find(d => d?.id === deviceId || d?.ersId === deviceId || d?.hdpId === deviceId)
      || rt.find(d => d?.id === deviceId || d?.hdpId === deviceId)
      || null
    );
  }, [deviceId, ersDevices, realtimeDevices]);

  const historyDeviceId = device?.hdpId || deviceId;

  const [series, setSeries] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [status, setStatus] = useState(null);

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

  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      if (!accessToken || !isResidentOrAdmin) {
        setStatus(accessToken ? 403 : 401);
        return;
      }
      if (!historyDeviceId || !metricKey) {
        setSeries([]);
        setError(null);
        setStatus(null);
        return;
      }

      setLoading(true);
      setError(null);
      setStatus(null);

      const now = new Date();
      const from = new Date(now.getTime() - rangeToMs(preset));

      try {
        const res = await listStatePoints(historyDeviceId, {
          from: toRFC3339(from),
          to: toRFC3339(now),
          limit: 400,
          order: 'asc',
        }, accessToken);

        if (cancelled) return;

        if (!res.success) {
          if (res.status === 401) setStatus(401);
          else if (res.status === 403) setStatus(403);
          else setError(res.error || 'Unable to load history');
          setSeries([]);
          return;
        }

        const points = Array.isArray(res.data?.points) ? res.data.points : [];
        const nextSeries = [];

        for (const p of points) {
          const ts = p?.ts;
          if (!ts) continue;

          let payload = p?.payload;
          if (typeof payload === 'string') {
            try {
              payload = JSON.parse(payload);
            } catch {
              payload = {};
            }
          }

          const root = payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : {};
          const rawState = root.state;
          const state = rawState && typeof rawState === 'object' && !Array.isArray(rawState)
            ? rawState
            : root;

          const value = resolveMetricValue(state, metricKey);
          if (value === undefined) continue;

          nextSeries.push({ ts, value });
        }

        setSeries(nextSeries);
      } catch (err) {
        if (!cancelled) {
          setError(err?.message || 'Unable to load history');
          setSeries([]);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    run();

    return () => {
      cancelled = true;
    };
  }, [accessToken, historyDeviceId, metricKey, preset, isResidentOrAdmin]);

  const title = settings.title || (metricKey ? `Graph: ${metricKey}` : 'Device Graph');
  const subtitle = device ? (device.displayName || device.name || device.hdpId || device.id || '') : '';

  if (!deviceId || !metricKey) {
    return (
      <WidgetShell
        title={title}
        subtitle={!deviceId ? 'No device selected' : 'No metric selected'}
        icon={faChartLine}
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="device-graph-widget"
      >
        <div className="device-graph-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="device-graph-widget__empty-icon" />
          <div className="device-graph-widget__empty-text">
            Open settings to choose a device and metric.
          </div>
        </div>
      </WidgetShell>
    );
  }

  return (
    <WidgetShell
      title={title}
      subtitle={subtitle}
      icon={faChartLine}
      loading={loading}
      error={error}
      status={status}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      className="device-graph-widget"
    >
      <div className="device-graph-widget__content">
        <HistoryChart
          title={metricKey}
          series={series}
          height={190}
          unit={unitForMetric(metricKey)}
        />
      </div>
    </WidgetShell>
  );
}

DeviceGraphWidget.defaultHeight = 5;
