import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Paho from 'paho-mqtt';

const DEVICEHUB_ROOT = 'homenavi/devicehub/';
const STATE_PREFIX = `${DEVICEHUB_ROOT}devices/`;
const EVENT_TOPIC = `${DEVICEHUB_ROOT}events/device.upsert`;
const REMOVED_TOPIC = `${DEVICEHUB_ROOT}events/device.removed`;
const PAIRING_TOPIC = `${DEVICEHUB_ROOT}events/pairing`;
const COMMAND_TOPIC = `${DEVICEHUB_ROOT}commands/device.set`;
const RENAME_TOPIC = `${DEVICEHUB_ROOT}commands/device.rename`;
const REALTIME_PATH = '/ws/devicehub';
const LEGACY_DEVICE_PREFIX = 'by-external/';

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

function isLegacyDeviceId(id) {
  return typeof id === 'string' && id.startsWith(LEGACY_DEVICE_PREFIX);
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
  const rawId = raw.id || raw.device_id || raw.deviceId;
  if (isLegacyDeviceId(rawId)) return false;
  if (typeof raw.mapKey === 'string' && isLegacyDeviceId(raw.mapKey)) return false;
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
  const externalId = raw.external_id || raw.externalId || raw.externalID || raw.external || key;
  const trimmedName = typeof raw.name === 'string' ? raw.name.trim() : '';
  const fallbackName = [raw.manufacturer, raw.model].filter(Boolean).join(' ') || externalId || key;
  const displayName = trimmedName || fallbackName;
  const icon = typeof raw.icon === 'string' ? raw.icon : (raw.metadata?.icon || '');
  return {
    key,
    mapKey: key,
    externalId,
    id: raw.id || raw.device_id || key,
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
      const legacyKeys = [];
      const nextDevices = entries
        .map(([id, raw]) => {
          if (isLegacyDeviceId(id)) {
            legacyKeys.push(id);
            return null;
          }
          if (!shouldIncludeDevice(raw)) {
            return null;
          }
          const entry = transformEntry(id, raw);
          if (!entry) {
            return null;
          }
          if (isLegacyDeviceId(entry.mapKey) || isLegacyDeviceId(entry.id)) {
            legacyKeys.push(id);
            return null;
          }
          return entry;
        })
        .filter(Boolean)
        .sort((a, b) => a.displayName.localeCompare(b.displayName, undefined, { sensitivity: 'base' }));
      if (legacyKeys.length) {
        legacyKeys.forEach(key => devicesRef.current.delete(key));
      }
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
      const response = await fetch('/api/devicehub/pairings', { credentials: 'include' });
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

  useEffect(() => {
    if (!enabledRef.current) {
      return;
    }
    refreshPairings();
  }, [enabled, refreshPairings]);

  const loadInitialDevices = useCallback(async () => {
    if (metadataMode !== 'rest') {
      setMetadataStatus({ connected: false, source: 'ws' });
      return;
    }
    try {
      const response = await fetch('/api/devicehub/devices', { credentials: 'include' });
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
        const deviceId = item.id || item.device_id;
        if (!deviceId) return;
        if (isLegacyDeviceId(deviceId)) {
          return;
        }
        const stateObj = ensureStateObject(item.state);
        next.set(deviceId, {
          ...item,
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

      if (topic === PAIRING_TOPIC) {
        const data = safeParseJSON(payload);
        const session = mapPairingSession(data);
        if (!session) return;
        setPairingSessions(prev => ({ ...prev, [session.protocol]: session }));
        return;
      }

      if (topic === EVENT_TOPIC) {
        const data = safeParseJSON(payload);
        if (!data || typeof data !== 'object') return;
        const deviceId = data.id || data.device_id || data.deviceId;
        if (!deviceId) return;
        if (isLegacyDeviceId(deviceId)) {
          const removed = devicesRef.current.delete(deviceId);
          if (removed) {
            schedulePublish();
          }
          return;
        }
        const prev = devicesRef.current.get(deviceId) || {};
        const merged = {
          ...prev,
          ...data,
          metadataUpdatedAt: Date.now(),
          __hasMetadata: true,
        };
        merged.capabilities = ensureArray(merged.capabilities);
        merged.inputs = ensureArray(merged.inputs);
        merged.description = normalizeDescription(merged.description);
        if (!merged.id) {
          merged.id = deviceId;
        }
        devicesRef.current.set(deviceId, merged);
        schedulePublish();
        return;
      }

      if (topic === REMOVED_TOPIC) {
        const data = safeParseJSON(payload);
        if (!data || typeof data !== 'object') return;
        const deviceId = data.id || data.device_id || data.deviceId;
        const externalId = data.external_id || data.externalId || data.externalID;
        let removed = false;
        if (deviceId) {
          removed = devicesRef.current.delete(deviceId);
        }
        if (!removed && externalId) {
          for (const [key, value] of devicesRef.current.entries()) {
            if (value?.externalId === externalId || value?.external_id === externalId || value?.externalID === externalId) {
              devicesRef.current.delete(key);
              removed = true;
              break;
            }
          }
        }
        if (removed) {
          schedulePublish();
        }
        return;
      }

      if (topic.startsWith(STATE_PREFIX)) {
        const deviceId = topic.slice(STATE_PREFIX.length);
        if (!deviceId) return;
        if (isLegacyDeviceId(deviceId)) {
          const removed = devicesRef.current.delete(deviceId);
          if (removed) {
            schedulePublish();
          }
          return;
        }
        const stateObj = ensureStateObject(safeParseJSON(payload));
        const prev = devicesRef.current.get(deviceId) || {};
        const merged = {
          ...prev,
          id: prev.id || deviceId,
          _last_state: stateObj,
          stateUpdatedAt: Date.now(),
          online: true,
        };
        if (prev.__hasMetadata) {
          merged.__hasMetadata = true;
        }
        devicesRef.current.set(deviceId, merged);
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
        client.subscribe(EVENT_TOPIC, { qos: 0 });
        client.subscribe(REMOVED_TOPIC, { qos: 0 });
        client.subscribe(PAIRING_TOPIC, { qos: 0 });
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
    try {
      const message = new Paho.Message(JSON.stringify({ device_id: deviceId, state: statePatch }));
      message.destinationName = COMMAND_TOPIC;
      message.qos = 0;
      message.retained = false;
      client.send(message);
      resolve();
    } catch (err) {
      reject(err);
    }
  }), []);

  const renameDevice = useCallback((deviceId, rawName) => new Promise((resolve, reject) => {
    const client = stateClientRef.current;
    if (!client || typeof client.isConnected !== 'function' || !client.isConnected()) {
      reject(new Error('Not connected to rename channel'));
      return;
    }
    const name = typeof rawName === 'string' ? rawName.trim() : '';
    if (!deviceId) {
      reject(new Error('Missing device id'));
      return;
    }
    if (!name) {
      reject(new Error('Name cannot be empty'));
      return;
    }
    try {
      const message = new Paho.Message(JSON.stringify({ device_id: deviceId, name }));
      message.destinationName = RENAME_TOPIC;
      message.qos = 0;
      message.retained = false;
      client.send(message);
      resolve();
    } catch (err) {
      reject(err);
    }
  }), []);

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
    refreshPairings,
  };
}
