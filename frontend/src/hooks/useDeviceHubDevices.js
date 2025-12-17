import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Paho from 'paho-mqtt';

const HDP_SCHEMA = 'hdp.v1';
const HDP_ROOT = 'homenavi/hdp/';
const METADATA_PREFIX = `${HDP_ROOT}device/metadata/`;
const STATE_PREFIX = `${HDP_ROOT}device/state/`;
const EVENT_PREFIX = `${HDP_ROOT}device/event/`;
const PAIRING_PREFIX = `${HDP_ROOT}pairing/progress/`;
const COMMAND_PREFIX = `${HDP_ROOT}device/command/`;
const COMMAND_RESULT_PREFIX = `${HDP_ROOT}device/command_result/`;
const REALTIME_PATH = '/ws/hdp';

const textDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder() : null;

function resolveGatewayUrl() {
  const override = import.meta.env.VITE_GATEWAY_ORIGIN;
  try {
    if (override) {
      return new URL(override);
    }
  } catch (err) {
    console.warn('Invalid VITE_GATEWAY_ORIGIN, falling back to window.origin', err);
  }
  return new URL(window.location.origin);
}

function joinPath(basePath, suffix) {
  const base = (!basePath || basePath === '/') ? '' : basePath.replace(/\/$/, '');
  const next = suffix.startsWith('/') ? suffix : `/${suffix}`;
  return `${base}${next}`;
}

function buildWsConfig(path) {
  const gateway = resolveGatewayUrl();
  const useSSL = gateway.protocol === 'https:' || gateway.protocol === 'wss:';
  const port = gateway.port ? Number(gateway.port) : (useSSL ? 443 : 80);
  const fullPath = joinPath(gateway.pathname, path);
  return {
    host: gateway.hostname,
    port,
    path: fullPath,
    useSSL,
  };
}

function safeParseJSON(value) {
  if (!value) return null;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return null;
    try {
      return JSON.parse(trimmed);
    } catch (err) {
      console.warn('Failed to parse JSON payload', trimmed.slice(0, 120), err);
      return null;
    }
  }
  if (typeof value === 'object') {
    return value;
  }
  return null;
}

function ensureArray(value) {
  if (Array.isArray(value)) {
    return value.map(item => (item && typeof item === 'object' ? { ...item } : item)).filter(Boolean);
  }
  if (value && typeof value === 'object') {
    return [{ ...value }];
  }
  if (typeof value === 'string') {
    const parsed = safeParseJSON(value);
    if (Array.isArray(parsed)) {
      return parsed;
    }
    if (parsed && typeof parsed === 'object') {
      return [parsed];
    }
  }
  return [];
}

function normalizeDescription(value) {
  if (typeof value === 'string') {
    return value.replace(/\s+/g, ' ').trim();
  }
  if (value && typeof value === 'object' && 'text' in value) {
    return normalizeDescription(value.text);
  }
  return '';
}

function ensureStateObject(value) {
  if (!value) return {};
  if (typeof value === 'string') {
    const parsed = safeParseJSON(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  }
  if (typeof value === 'object') {
    return { ...value };
  }
  return {};
}

function normalizeDeviceId(raw) {
  const deviceId = (raw || '').trim();
  if (!deviceId) {
    return { external: '', hdpId: '', protocol: '', adapter: '' };
  }
  const parts = deviceId.split('/').filter(Boolean);
  if (parts.length === 0) {
    return { external: deviceId, hdpId: deviceId, protocol: '', adapter: '' };
  }
  const protocol = parts[0];
  if (parts.length === 1) {
    return { external: deviceId, hdpId: deviceId, protocol, adapter: '' };
  }

  const rest = parts.slice(1).join('/');
  return { external: rest, hdpId: `${protocol}/${rest}`, protocol, adapter: '' };
}

function mapPairingSession(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const protocol = typeof raw.protocol === 'string' ? raw.protocol.toLowerCase() : '';
  if (!protocol) return null;
  const normalizeDate = value => {
    if (!value) return null;
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? null : d;
  };
  return {
    id: raw.id || `${protocol}-${raw.started_at || Date.now()}`,
    protocol,
    status: raw.status || 'unknown',
    active: Boolean(raw.active),
    startedAt: normalizeDate(raw.started_at),
    expiresAt: normalizeDate(raw.expires_at),
    deviceId: raw.device_id || raw.deviceId || '',
    metadata: raw.metadata && typeof raw.metadata === 'object' ? { ...raw.metadata } : {},
  };
}

function sessionsArrayToMap(payload) {
  if (!Array.isArray(payload)) return {};
  const next = {};
  payload.forEach(item => {
    const session = mapPairingSession(item);
    if (session) {
      next[session.protocol] = session;
    }
  });
  return next;
}

export function pairingConfigArrayToMap(payload) {
  if (!Array.isArray(payload)) return {};
  const next = {};
  payload.forEach(item => {
    if (item && typeof item.protocol === 'string') {
      const key = item.protocol.toLowerCase();
      next[key] = item;
    }
  });
  return next;
}

function toBoolean(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['on', 'true', '1', 'yes', 'enabled'].includes(normalized)) return true;
    if (['off', 'false', '0', 'no', 'disabled'].includes(normalized)) return false;
  }
  return false;
}

function shouldIncludeDevice(raw) {
  if (!raw || typeof raw !== 'object') return false;
  if (raw.__hasMetadata) return true;
  if (raw.metadataUpdatedAt) return true;
  if (raw.name || raw.manufacturer || raw.model || raw.description || raw.protocol) return true;
  if (ensureArray(raw.capabilities).length > 0) return true;
  if (ensureArray(raw.inputs).length > 0) return true;
  return Object.keys(ensureStateObject(raw._last_state)).length > 0;
}

function transformEntry(key, raw) {
  if (!raw || typeof raw !== 'object') {
    return null;
  }
  const stateObj = ensureStateObject(raw._last_state);
  const capabilities = ensureArray(raw.capabilities);
  const inputs = ensureArray(raw.inputs);
  const lastSeen = raw.last_seen ? new Date(raw.last_seen) : null;
  const metadataUpdatedAt = raw.metadataUpdatedAt ? new Date(raw.metadataUpdatedAt) : null;
  const stateUpdatedAt = raw.stateUpdatedAt ? new Date(raw.stateUpdatedAt) : null;
  const createdAt = raw.created_at ? new Date(raw.created_at) : null;
  const updatedAt = raw.updated_at ? new Date(raw.updated_at) : null;
  const lastCommandResult = raw.__lastCommandResult || null;
  const hasToggle = capabilities.some(cap => {
    const id = (cap.id || cap.property || '').toLowerCase();
    const valueType = (cap.value_type || cap.valueType || '').toLowerCase();
    const kind = (cap.kind || '').toLowerCase();
    return id === 'state' || id === 'on' || valueType === 'boolean' || kind === 'binary';
  });
  const toggleState = (() => {
    if ('state' in stateObj) return toBoolean(stateObj.state);
    if ('on' in stateObj) return toBoolean(stateObj.on);
    return null;
  })();
  const externalId = raw.device_id || raw.deviceId || raw.external_id || raw.externalId || raw.externalID || raw.external || key;
  const hdpId = raw.hdpId || raw.device_id || raw.deviceId || externalId || key;
  const trimmedName = typeof raw.name === 'string' ? raw.name.trim() : '';
  const fallbackName = [raw.manufacturer, raw.model].filter(Boolean).join(' ') || externalId || key;
  const displayName = trimmedName || fallbackName;
  const icon = typeof raw.icon === 'string' ? raw.icon : (raw.metadata?.icon || '');
  return {
    key,
    mapKey: key,
    externalId,
    hdpId,
    id: hdpId,
    protocol: raw.protocol || '',
    name: trimmedName,
    displayName,
    type: raw.type || '',
    manufacturer: raw.manufacturer || '',
    model: raw.model || '',
    firmware: raw.firmware || raw.software_build_id || '',
    description: normalizeDescription(raw.description),
    icon,
    capabilities,
    inputs,
    online: Boolean(raw.online),
    lastSeen,
    createdAt,
    updatedAt,
    metadataUpdatedAt,
    state: stateObj,
    stateUpdatedAt,
    stateHasValues: Object.keys(stateObj).length > 0,
    hasToggle,
    toggleState,
    lastCommandResult,
  };
}

function computeStats(devices) {
  const total = devices.length;
  if (total === 0) {
    return { total: 0, online: 0, withState: 0, sensors: 0 };
  }
  let online = 0;
  let withState = 0;
  let sensors = 0;
  devices.forEach(dev => {
    if (dev.online) online += 1;
    if (dev.stateHasValues) withState += 1;
    if (dev.capabilities.some(cap => (cap.kind || '').toLowerCase() === 'numeric')) sensors += 1;
  });
  return { total, online, withState, sensors };
}

export default function useDeviceHubDevices(options = {}) {
  const { enabled = true, metadataMode: rawMetadataMode = 'rest' } = options;
  const metadataMode = rawMetadataMode === 'ws' ? 'ws' : 'rest';
  const [devices, setDevices] = useState([]);
  const [stats, setStats] = useState({ total: 0, online: 0, withState: 0, sensors: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [metadataStatus, setMetadataStatus] = useState(() => ({ connected: false, source: metadataMode }));
  const [stateStatus, setStateStatus] = useState({ connected: false });
  const [pairingSessions, setPairingSessions] = useState({});
  const [pairingConfig, setPairingConfig] = useState({});

  const devicesRef = useRef(new Map());
  const stateClientRef = useRef(null);
  const updateScheduledRef = useRef(false);
  const mountedRef = useRef(false);
  const enabledRef = useRef(enabled);

  useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    setMetadataStatus({ connected: false, source: metadataMode });
  }, [metadataMode]);

  const schedulePublish = useCallback(() => {
    if (updateScheduledRef.current || !enabledRef.current) return;
    updateScheduledRef.current = true;
    const publish = () => {
      updateScheduledRef.current = false;
      if (!mountedRef.current || !enabledRef.current) {
        return;
      }
      const entries = Array.from(devicesRef.current.entries());
      const nextDevices = [];
      entries.forEach(([id, raw]) => {
        if (!shouldIncludeDevice(raw)) return;
        const entry = transformEntry(id, raw);
        if (entry) {
          nextDevices.push(entry);
        }
      });
      nextDevices.sort((a, b) => a.displayName.localeCompare(b.displayName, undefined, { sensitivity: 'base' }));
      setDevices(nextDevices);
      setStats(computeStats(nextDevices));
      setLoading(false);
    };
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(publish);
    } else {
      setTimeout(publish, 16);
    }
  }, []);

  const refreshPairings = useCallback(async () => {
    try {
      const response = await fetch('/api/hdp/pairings', { credentials: 'include' });
      if (!response.ok) {
        throw new Error(`pairing request failed with status ${response.status}`);
      }
      const payload = await response.json();
      if (!mountedRef.current) {
        return;
      }
      setPairingSessions(sessionsArrayToMap(payload));
    } catch (err) {
      console.warn('Pairing status fetch failed', err);
    }
  }, []);

  const refreshPairingConfig = useCallback(async () => {
    try {
      const response = await fetch('/api/hdp/pairing-config', { credentials: 'include' });
      if (!response.ok) {
        throw new Error(`pairing config request failed with status ${response.status}`);
      }
      const payload = await response.json();
      if (!mountedRef.current) {
        return;
      }
      if (Array.isArray(payload)) {
        setPairingConfig(pairingConfigArrayToMap(payload));
      }
    } catch (err) {
      console.warn('Pairing config fetch failed', err);
    }
  }, []);

  useEffect(() => {
    if (!enabledRef.current) {
      return;
    }
    refreshPairings();
    refreshPairingConfig();
  }, [enabled, refreshPairings, refreshPairingConfig]);

  const loadInitialDevices = useCallback(async () => {
    if (metadataMode !== 'rest') {
      setMetadataStatus({ connected: false, source: 'ws' });
      return;
    }
    try {
      const response = await fetch('/api/hdp/devices', { credentials: 'include' });
      if (!response.ok) {
        throw new Error(`request failed with status ${response.status}`);
      }
      const payload = await response.json();
      if (!Array.isArray(payload)) {
        throw new Error('invalid device list payload');
      }
      const now = Date.now();
      const next = new Map();
      payload.forEach(item => {
        if (!item || typeof item !== 'object') return;
        const idFromApi = item.device_id || item.external_id || item.id;
        const norm = normalizeDeviceId(idFromApi);
        const mapKey = norm.hdpId || idFromApi;
        if (!mapKey) return;
        const stateObj = ensureStateObject(item.state);
        next.set(mapKey, {
          ...item,
          id: mapKey,
          mapKey,
          device_id: mapKey,
          externalId: mapKey,
          hdpId: mapKey,
          icon: typeof item.icon === 'string' ? item.icon : (item.metadata?.icon || ''),
          capabilities: ensureArray(item.capabilities),
          inputs: ensureArray(item.inputs),
          description: normalizeDescription(item.description),
          _last_state: stateObj,
          metadataUpdatedAt: now,
          stateUpdatedAt: Object.keys(stateObj).length > 0 ? now : null,
          __hasMetadata: true,
        });
      });
      if (!mountedRef.current || !enabledRef.current) {
        return;
      }
      devicesRef.current = next;
      setMetadataStatus({ connected: true, source: 'rest' });
      setError(prev => (prev && prev.includes('device list') ? null : prev));
      schedulePublish();
    } catch (err) {
      console.warn('Device list fetch failed', err);
      if (!mountedRef.current || !enabledRef.current) {
        return;
      }
      setMetadataStatus({ connected: false, source: 'rest' });
      setError(prev => prev || 'Unable to load device list');
      setLoading(false);
    }
  }, [metadataMode, schedulePublish]);

  const connectRealtime = useCallback(() => {
    if (!enabledRef.current) {
      return;
    }
    const cfg = buildWsConfig(REALTIME_PATH);
    const client = new Paho.Client(cfg.host, Number(cfg.port), cfg.path, `rt-${Date.now()}-${Math.floor(Math.random() * 1000)}`);
    stateClientRef.current = client;

    client.onConnectionLost = response => {
      console.warn('Realtime connection lost', response);
      setStateStatus({ connected: false });
      if (metadataMode === 'ws') {
        setMetadataStatus({ connected: false, source: 'ws' });
      }
      if (!enabledRef.current) {
        return;
      }
      setTimeout(() => {
        if (enabledRef.current) {
          connectRealtime();
        }
      }, 1500);
    };

    client.onMessageArrived = message => {
      if (!enabledRef.current || !message || !message.destinationName) return;
      const topic = message.destinationName;
      const payload = typeof message.payloadString === 'string'
        ? message.payloadString
        : (textDecoder && message.payloadBytes ? textDecoder.decode(message.payloadBytes) : '');

      if (topic.startsWith(PAIRING_PREFIX)) {
        const data = safeParseJSON(payload) || {};
        const protocol = (data.protocol || topic.slice(PAIRING_PREFIX.length) || '').toLowerCase();
        if (!protocol) return;
        const status = data.stage || data.status || 'in_progress';
        const session = {
          id: data.id || `${protocol}-${Date.now()}`,
          protocol,
          status,
          active: status !== 'completed' && status !== 'failed' && status !== 'timeout' && status !== 'stopped',
          metadata: data.metadata && typeof data.metadata === 'object' ? { ...data.metadata } : {},
        };
        setPairingSessions(prev => ({ ...prev, [protocol]: session }));
        return;
      }

      if (topic.startsWith(METADATA_PREFIX)) {
        const data = safeParseJSON(payload);
        if (!data || typeof data !== 'object') return;
        const norm = normalizeDeviceId(data.device_id || data.deviceId || topic.slice(METADATA_PREFIX.length));
        const mapKey = norm.hdpId || '';
        if (!mapKey) return;
        const prev = devicesRef.current.get(mapKey) || {};
        const online = typeof data.online === 'boolean' ? data.online : prev.online;
        const lastSeen = data.last_seen ?? data.lastSeen ?? prev.last_seen;
        const merged = {
          ...prev,
          id: mapKey,
          mapKey,
          device_id: mapKey,
          externalId: mapKey,
          hdpId: mapKey,
          protocol: data.protocol || prev.protocol || norm.protocol || (mapKey.includes('/') ? mapKey.split('/')[0] : ''),
          name: data.name ?? prev.name,
          manufacturer: data.manufacturer ?? prev.manufacturer,
          model: data.model ?? prev.model,
          firmware: data.firmware ?? prev.firmware,
          description: normalizeDescription(data.description ?? prev.description),
          icon: data.icon ?? prev.icon,
          capabilities: ensureArray(data.capabilities ?? prev.capabilities),
          inputs: ensureArray(data.inputs ?? prev.inputs),
          online: typeof online === 'boolean' ? online : Boolean(prev.online),
          last_seen: lastSeen,
          metadataUpdatedAt: Date.now(),
          __hasMetadata: true,
        };
        devicesRef.current.set(mapKey, merged);
        schedulePublish();
        return;
      }

      if (topic.startsWith(EVENT_PREFIX)) {
        const data = safeParseJSON(payload);
        if (!data || typeof data !== 'object') return;
        const norm = normalizeDeviceId(data.device_id || data.deviceId || topic.slice(EVENT_PREFIX.length));
        const mapKey = norm.hdpId || '';
        if (!mapKey) return;
        if (data.event === 'device_removed') {
          const removed = devicesRef.current.delete(mapKey);
          if (removed) {
            schedulePublish();
          }
        }
        return;
      }

      if (topic.startsWith(COMMAND_RESULT_PREFIX)) {
        const data = safeParseJSON(payload) || {};
        const norm = normalizeDeviceId(data.device_id || data.deviceId || topic.slice(COMMAND_RESULT_PREFIX.length));
        const mapKey = norm.hdpId || '';
        if (!mapKey) return;
        const result = {
          corr: data.corr || data.correlation_id || '',
          success: typeof data.success === 'boolean' ? data.success : true,
          status: data.status || '',
          error: data.error || null,
          ts: data.ts || Date.now(),
        };
        const prev = devicesRef.current.get(mapKey) || {};
        devicesRef.current.set(mapKey, { ...prev, id: mapKey, mapKey, device_id: mapKey, externalId: mapKey, hdpId: mapKey, __lastCommandResult: result });
        schedulePublish();
        return;
      }

      if (topic.startsWith(STATE_PREFIX)) {
        const stateEnvelope = safeParseJSON(payload) || {};
        const norm = normalizeDeviceId(stateEnvelope.device_id || topic.slice(STATE_PREFIX.length));
        const mapKey = norm.hdpId || '';
        if (!mapKey) return;
        const stateObj = ensureStateObject(stateEnvelope.state || stateEnvelope.data || stateEnvelope);
        if (!Object.keys(stateObj).length) return;
        const prev = devicesRef.current.get(mapKey) || {};
        const incomingTs = Number(stateEnvelope.ts || stateEnvelope.timestamp || Date.now());
        const prevTs = prev.stateUpdatedAt instanceof Date
          ? prev.stateUpdatedAt.getTime()
          : typeof prev.stateUpdatedAt === 'number'
            ? prev.stateUpdatedAt
            : 0;
        if (prevTs && incomingTs && incomingTs < prevTs) {
          // Drop stale state to avoid UI flicker (older MQTT state overriding optimistic UI).
          return;
        }
        const merged = {
          ...prev,
          id: mapKey,
          mapKey,
          device_id: mapKey,
          externalId: mapKey,
          hdpId: mapKey,
          _last_state: stateObj,
          last_seen: incomingTs || Date.now(),
          stateUpdatedAt: incomingTs || Date.now(),
          online: true,
        };
        if (prev.__hasMetadata) {
          merged.__hasMetadata = true;
        }
        devicesRef.current.set(mapKey, merged);
        schedulePublish();
      }
    };

    client.connect({
      useSSL: cfg.useSSL,
      timeout: 6,
      mqttVersion: 4,
      cleanSession: true,
      onSuccess: () => {
        if (!mountedRef.current || !enabledRef.current) {
          client.disconnect();
          return;
        }
        client.subscribe(STATE_PREFIX + '#', { qos: 0 });
        client.subscribe(METADATA_PREFIX + '#', { qos: 0 });
        client.subscribe(EVENT_PREFIX + '#', { qos: 0 });
        client.subscribe(PAIRING_PREFIX + '#', { qos: 0 });
        client.subscribe(COMMAND_RESULT_PREFIX + '#', { qos: 0 });
        setStateStatus({ connected: true });
        if (metadataMode === 'ws') {
          setMetadataStatus({ connected: true, source: 'ws' });
          setLoading(false);
        }
      },
      onFailure: err => {
        console.warn('Realtime connection failed', err);
        client.disconnect();
        setStateStatus({ connected: false });
        if (metadataMode === 'ws') {
          setMetadataStatus({ connected: false, source: 'ws' });
          setLoading(false);
        }
        setError(prev => prev || 'Unable to connect to device stream');
        if (!enabledRef.current) {
          return;
        }
        setTimeout(() => {
          if (enabledRef.current) {
            connectRealtime();
          }
        }, 2000);
      },
    });
  }, [metadataMode, schedulePublish]);

  useEffect(() => {
    devicesRef.current = new Map();

    updateScheduledRef.current = false;

    if (!enabled) {
      stateClientRef.current?.disconnect?.();
      stateClientRef.current = null;
      setDevices([]);
      setStats({ total: 0, online: 0, withState: 0, sensors: 0 });
      setMetadataStatus({ connected: false, source: metadataMode });
      setStateStatus({ connected: false });
      setLoading(false);
      setError(null);
      return undefined;
    }

    setDevices([]);
    setStats({ total: 0, online: 0, withState: 0, sensors: 0 });
    setMetadataStatus({ connected: false, source: metadataMode });
    setStateStatus({ connected: false });
    setError(null);
    setLoading(true);

    loadInitialDevices();
    connectRealtime();

    return () => {
      stateClientRef.current?.disconnect?.();
      stateClientRef.current = null;
    };
  }, [enabled, loadInitialDevices, connectRealtime]);

  const sendDeviceCommand = useCallback((deviceId, statePatch) => new Promise((resolve, reject) => {
    const client = stateClientRef.current;
    if (!client || typeof client.isConnected !== 'function' || !client.isConnected()) {
      reject(new Error('Not connected to device command channel'));
      return;
    }
    if (!deviceId || !statePatch || typeof statePatch !== 'object') {
      reject(new Error('Invalid command payload'));
      return;
    }
    const resolveTargetId = () => {
      const direct = devicesRef.current.get(deviceId);
      if (direct) {
        return direct.hdpId || direct.externalId || deviceId;
      }
      for (const [, dev] of devicesRef.current.entries()) {
        if (dev.id === deviceId || dev.mapKey === deviceId || dev.externalId === deviceId) {
          return dev.hdpId || dev.externalId || dev.id || deviceId;
        }
      }
      return deviceId;
    };
    const targetId = resolveTargetId();
    try {
      const corr = `cmd-${Date.now()}-${Math.floor(Math.random() * 1000)}`;
      const envelope = {
        schema: HDP_SCHEMA,
        type: 'command',
        device_id: targetId,
        command: 'set_state',
        args: statePatch,
        ts: Date.now(),
        corr,
      };
      const message = new Paho.Message(JSON.stringify(envelope));
      message.destinationName = `${COMMAND_PREFIX}${targetId}`;
      message.qos = 0;
      message.retained = false;
      client.send(message);
      resolve();
    } catch (err) {
      reject(err);
    }
  }), []);

  const renameDevice = useCallback(() => Promise.reject(new Error('Rename over MQTT is disabled; use HTTP fallback')), []);

  const connectionInfo = useMemo(() => ({
    metadata: metadataStatus,
    state: stateStatus,
  }), [metadataStatus, stateStatus]);

  return {
    devices,
    stats,
    loading,
    error,
    connectionInfo,
    sendDeviceCommand,
    renameDevice,
    pairingSessions,
    pairingConfig,
    refreshPairings,
  };
}
