import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { getSharedMqttConnection } from '../services/realtime/sharedMqtt';

const HDP_SCHEMA = 'hdp.v1';
const HDP_ROOT = 'homenavi/hdp/';
const METADATA_PREFIX = `${HDP_ROOT}device/metadata/`;
const STATE_PREFIX = `${HDP_ROOT}device/state/`;
const EVENT_PREFIX = `${HDP_ROOT}device/event/`;
const PAIRING_PREFIX = `${HDP_ROOT}pairing/progress/`;
const COMMAND_PREFIX = `${HDP_ROOT}device/command/`;
const COMMAND_RESULT_PREFIX = `${HDP_ROOT}device/command_result/`;
const REALTIME_PATH = '/ws/hdp';
const REALTIME_SUBSCRIPTION_FILTERS = [
  METADATA_PREFIX + '#',
  STATE_PREFIX + '#',
  EVENT_PREFIX + '#',
  PAIRING_PREFIX + '#',
  COMMAND_RESULT_PREFIX + '#',
];

const CANONICAL_ZIGBEE_ID_RE = /^zigbee\/0x[0-9a-f]{16}$/i;

function isCanonicalZigbeeId(hdpId) {
  return CANONICAL_ZIGBEE_ID_RE.test(String(hdpId || '').trim());
}

const textDecoder = typeof TextDecoder !== 'undefined' ? new TextDecoder() : null;

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
  if (raw.manufacturer || raw.model || raw.description || raw.protocol) return true;
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
  const lastStateCorr = typeof raw.__lastStateCorr === 'string' ? raw.__lastStateCorr : '';
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
  const icon = typeof raw.icon === 'string' ? raw.icon : (raw.metadata?.icon || '');
  return {
    key,
    mapKey: key,
    externalId,
    hdpId,
    id: hdpId,
    protocol: raw.protocol || '',
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
    lastStateCorr,
  };
}

export function mergeMetadataRecord(prev, data, mapKey, protocolHint = '') {
  const online = typeof data?.online === 'boolean' ? data.online : prev?.online;
  const lastSeen = data?.last_seen ?? data?.lastSeen ?? prev?.last_seen;
  const pendingState = ensureStateObject(prev?.__pendingState);
  const pendingStateTs = Number(prev?.__pendingStateTs || 0);
  const pendingStateCorr = typeof prev?.__pendingStateCorr === 'string' ? prev.__pendingStateCorr : '';
  const mergedState = Object.keys(pendingState).length > 0
    ? pendingState
    : ensureStateObject(prev?._last_state);

  const merged = {
    ...(prev || {}),
    id: mapKey,
    mapKey,
    device_id: mapKey,
    externalId: mapKey,
    hdpId: mapKey,
    protocol: data?.protocol || prev?.protocol || protocolHint || (mapKey.includes('/') ? mapKey.split('/')[0] : ''),
    manufacturer: data?.manufacturer ?? prev?.manufacturer,
    model: data?.model ?? prev?.model,
    firmware: data?.firmware ?? prev?.firmware,
    description: normalizeDescription(data?.description ?? prev?.description),
    icon: data?.icon ?? prev?.icon,
    capabilities: ensureArray(data?.capabilities ?? prev?.capabilities),
    inputs: ensureArray(data?.inputs ?? prev?.inputs),
    online: typeof online === 'boolean' ? online : Boolean(prev?.online),
    last_seen: lastSeen,
    _last_state: mergedState,
    stateUpdatedAt: pendingStateTs || prev?.stateUpdatedAt || null,
    metadataUpdatedAt: Date.now(),
    __hasMetadata: true,
  };
  if (pendingStateCorr) {
    merged.__lastStateCorr = pendingStateCorr;
  }
  delete merged.__pendingState;
  delete merged.__pendingStateTs;
  delete merged.__pendingStateCorr;
  return merged;
}

export function mergeStateRecord(prev, mapKey, stateEnvelope) {
  const stateObj = ensureStateObject(stateEnvelope?.state || stateEnvelope?.data || stateEnvelope);
  if (!Object.keys(stateObj).length) return null;

  const incomingCorr = typeof stateEnvelope?.corr === 'string' ? stateEnvelope.corr.trim() : '';
  const incomingTs = Number(stateEnvelope?.ts || stateEnvelope?.timestamp || Date.now());
  const previous = prev || {};

  if (mapKey.startsWith('zigbee/') && !previous.__hasMetadata) {
    const prevPendingTs = Number(previous.__pendingStateTs || 0);
    if (prevPendingTs && incomingTs && incomingTs < prevPendingTs) {
      return null;
    }
    return {
      ...previous,
      id: mapKey,
      mapKey,
      device_id: mapKey,
      externalId: mapKey,
      hdpId: mapKey,
      __pendingState: stateObj,
      __pendingStateTs: incomingTs || Date.now(),
      __pendingStateCorr: incomingCorr,
      last_seen: incomingTs || Date.now(),
      online: true,
    };
  }

  const prevTs = previous.stateUpdatedAt instanceof Date
    ? previous.stateUpdatedAt.getTime()
    : typeof previous.stateUpdatedAt === 'number'
      ? previous.stateUpdatedAt
      : 0;
  if (prevTs && incomingTs && incomingTs < prevTs) {
    return null;
  }
  const merged = {
    ...previous,
    id: mapKey,
    mapKey,
    device_id: mapKey,
    externalId: mapKey,
    hdpId: mapKey,
    _last_state: stateObj,
    last_seen: incomingTs || Date.now(),
    stateUpdatedAt: incomingTs || Date.now(),
    online: true,
    __lastStateCorr: incomingCorr,
  };
  if (previous.__hasMetadata) {
    merged.__hasMetadata = true;
  }
  return merged;
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

function nowMs() {
  if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
    return performance.now();
  }
  return Date.now();
}

function createRealtimeInitMetrics() {
  return {
    authReadyMs: null,
    socketOpenMs: null,
    subscribeCompleteMs: null,
    firstStateReceivedMs: null,
  };
}

export default function useDeviceHubDevices(options = {}) {
  const {
    enabled = true,
    metadataMode: rawMetadataMode = 'rest',
    accessToken = '',
    authReady = false,
  } = options;
  const metadataMode = rawMetadataMode === 'ws' ? 'ws' : 'rest';
  const [devices, setDevices] = useState([]);
  const [stats, setStats] = useState({ total: 0, online: 0, withState: 0, sensors: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [metadataStatus, setMetadataStatus] = useState(() => ({ connected: false, source: metadataMode }));
  const [stateStatus, setStateStatus] = useState({ connected: false, subscribed: false, firstStateReceived: false });
  const [realtimeMetrics, setRealtimeMetrics] = useState(createRealtimeInitMetrics);
  const [pairingSessions, setPairingSessions] = useState({});
  const [pairingConfig, setPairingConfig] = useState({});

  const devicesRef = useRef(new Map());
  const mqttConnRef = useRef(null);
  const updateScheduledRef = useRef(false);
  const mountedRef = useRef(false);
  const enabledRef = useRef(enabled);
  const realtimeInitStartedAtRef = useRef(0);
  const subscriptionAcksRef = useRef(new Set());

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

  const resetRealtimeInitMetrics = useCallback(() => {
    realtimeInitStartedAtRef.current = nowMs();
    subscriptionAcksRef.current = new Set();
    setRealtimeMetrics(createRealtimeInitMetrics());
    setStateStatus({ connected: false, subscribed: false, firstStateReceived: false });
  }, []);

  const markRealtimeMetric = useCallback((key) => {
    const startedAt = realtimeInitStartedAtRef.current;
    if (!startedAt) return;
    const elapsed = Math.max(0, Math.round(nowMs() - startedAt));
    setRealtimeMetrics((prev) => {
      if (prev[key] != null) return prev;
      return { ...prev, [key]: elapsed };
    });
  }, []);

  useEffect(() => {
    if (!enabled || !authReady) return;
    markRealtimeMetric('authReadyMs');
  }, [authReady, enabled, markRealtimeMetric]);

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
        if (String(id || '').startsWith('zigbee/') && !isCanonicalZigbeeId(id)) return;
        if (!shouldIncludeDevice(raw)) return;
        const entry = transformEntry(id, raw);
        if (entry) {
          nextDevices.push(entry);
        }
      });
      nextDevices.sort((a, b) => {
        const aKey = String(a?.hdpId || a?.externalId || a?.id || '').toLowerCase();
        const bKey = String(b?.hdpId || b?.externalId || b?.id || '').toLowerCase();
        return aKey.localeCompare(bKey, undefined, { sensitivity: 'base' });
      });
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

  const buildFetchOptions = useCallback(() => {
    const headers = {};
    if (typeof accessToken === 'string' && accessToken.trim()) {
      headers.Authorization = `Bearer ${accessToken.trim()}`;
    }
    return Object.keys(headers).length > 0
      ? { credentials: 'include', headers }
      : { credentials: 'include' };
  }, [accessToken]);

  const refreshPairings = useCallback(async () => {
    try {
      const response = await fetch('/api/hdp/pairings', buildFetchOptions());
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
  }, [buildFetchOptions]);

  const refreshPairingConfig = useCallback(async () => {
    try {
      const response = await fetch('/api/hdp/pairing-config', buildFetchOptions());
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
  }, [buildFetchOptions]);

  useEffect(() => {
    if (!enabledRef.current) {
      return;
    }
    refreshPairings();
    refreshPairingConfig();
  }, [enabled, refreshPairings, refreshPairingConfig]);

  const loadInitialDevices = useCallback(async ({ silent = false } = {}) => {
    try {
      const response = await fetch('/api/hdp/devices', buildFetchOptions());
      if (!response.ok) {
        throw new Error(`request failed with status ${response.status}`);
      }
      const payload = await response.json();
      if (!Array.isArray(payload)) {
        throw new Error('invalid device list payload');
      }
      const now = Date.now();
      const next = new Map();
      const previousEntries = devicesRef.current;
      payload.forEach(item => {
        if (!item || typeof item !== 'object') return;
        const idFromApi = item.device_id || item.external_id || item.id;
        const norm = normalizeDeviceId(idFromApi);
        const mapKey = norm.hdpId || idFromApi;
        if (!mapKey) return;

        // Defense-in-depth: never surface non-canonical zigbee ids.
        if (String(mapKey).startsWith('zigbee/') && !isCanonicalZigbeeId(mapKey)) {
          return;
        }

        const stateObj = ensureStateObject(item.state);
        const prev = previousEntries.get(mapKey) || {};
        next.set(mapKey, {
          ...prev,
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
          _last_state: Object.keys(stateObj).length > 0 ? stateObj : ensureStateObject(prev._last_state),
          metadataUpdatedAt: now,
          stateUpdatedAt: Object.keys(stateObj).length > 0 ? now : (prev.stateUpdatedAt ?? null),
          __hasMetadata: true,
        });
      });
      if (!mountedRef.current || !enabledRef.current) {
        return;
      }
      devicesRef.current = next;
      if (metadataMode === 'rest') {
        setMetadataStatus({ connected: true, source: 'rest' });
      }
      setError(prev => {
        if (!prev) return prev;
        if (prev.includes('device list') || prev.includes('device stream')) return null;
        return prev;
      });
      schedulePublish();
    } catch (err) {
      console.warn('Device list fetch failed', err);
      if (!mountedRef.current || !enabledRef.current) {
        return;
      }
      if (!silent && metadataMode === 'rest') {
        setMetadataStatus({ connected: false, source: 'rest' });
        setError(prev => prev || 'Unable to load device list');
        setLoading(false);
      }
    }
  }, [buildFetchOptions, metadataMode, schedulePublish]);

  const handleRealtimeMessage = useCallback(({ topic, payloadString, payloadBytes }) => {
    if (!enabledRef.current || !topic) return;

    const payload = typeof payloadString === 'string' && payloadString
      ? payloadString
      : (textDecoder && payloadBytes ? textDecoder.decode(payloadBytes) : '');

    if (topic.startsWith(PAIRING_PREFIX)) {
      const data = safeParseJSON(payload) || {};
      const origin = typeof data.origin === 'string' ? data.origin.toLowerCase() : '';
      if (origin && origin !== 'device-hub') {
        return;
      }
      if (!origin) {
        return;
      }
      const protocol = (data.protocol || topic.slice(PAIRING_PREFIX.length) || '').toLowerCase();
      if (!protocol) return;
      const status = data.stage || data.status || 'in_progress';
      const sessionId = data.id || protocol;
      const isTerminal = ['completed', 'failed', 'timeout', 'stopped', 'error'].includes(String(status).toLowerCase());
      const isActive = !isTerminal;
      const session = {
        id: sessionId,
        protocol,
        status,
        active: isActive,
        metadata: data.metadata && typeof data.metadata === 'object' ? { ...data.metadata } : {},
      };
      setPairingSessions(prev => {
        const existing = prev?.[protocol];
        if (existing?.active && isTerminal && sessionId && existing.id && sessionId !== existing.id) {
          return prev;
        }
        return { ...prev, [protocol]: session };
      });
      return;
    }

    if (topic.startsWith(METADATA_PREFIX)) {
      const data = safeParseJSON(payload);
      if (!data || typeof data !== 'object') return;
      const norm = normalizeDeviceId(data.device_id || data.deviceId || topic.slice(METADATA_PREFIX.length));
      const mapKey = norm.hdpId || '';
      if (!mapKey) return;

      if (String(mapKey).startsWith('zigbee/') && !isCanonicalZigbeeId(mapKey)) {
        return;
      }

      const prev = devicesRef.current.get(mapKey) || {};
      const merged = mergeMetadataRecord(prev, data, mapKey, norm.protocol);
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
        origin: data.origin || '',
        terminal: data.terminal === true,
        ts: data.ts || Date.now(),
      };
      const prev = devicesRef.current.get(mapKey) || {};
      const prevResult = prev.__lastCommandResult || null;
      const shouldKeepPrev = prevResult
        && prevResult.origin === 'device-hub'
        && result.origin !== 'device-hub'
        && prevResult.corr
        && prevResult.corr === result.corr;
      devicesRef.current.set(mapKey, {
        ...prev,
        id: mapKey,
        mapKey,
        device_id: mapKey,
        externalId: mapKey,
        hdpId: mapKey,
        __lastCommandResult: shouldKeepPrev ? prevResult : result,
      });
      schedulePublish();
      return;
    }

    if (topic.startsWith(STATE_PREFIX)) {
      markRealtimeMetric('firstStateReceivedMs');
      setStateStatus((prev) => (prev.firstStateReceived ? prev : { ...prev, firstStateReceived: true }));
      const stateEnvelope = safeParseJSON(payload) || {};
      const norm = normalizeDeviceId(stateEnvelope.device_id || topic.slice(STATE_PREFIX.length));
      const mapKey = norm.hdpId || '';
      if (!mapKey) return;

      if (String(mapKey).startsWith('zigbee/') && !isCanonicalZigbeeId(mapKey)) {
        return;
      }

      const prev = devicesRef.current.get(mapKey) || {};
      const merged = mergeStateRecord(prev, mapKey, stateEnvelope);
      if (!merged) return;
      devicesRef.current.set(mapKey, merged);
      schedulePublish();
    }
  }, [markRealtimeMetric, schedulePublish]);

  useEffect(() => {
    devicesRef.current = new Map();

    updateScheduledRef.current = false;

    if (!enabled) {
      mqttConnRef.current = null;
      setDevices([]);
      setStats({ total: 0, online: 0, withState: 0, sensors: 0 });
      setMetadataStatus({ connected: false, source: metadataMode });
      setStateStatus({ connected: false, subscribed: false, firstStateReceived: false });
      setRealtimeMetrics(createRealtimeInitMetrics());
      setLoading(false);
      setError(null);
      return undefined;
    }

    resetRealtimeInitMetrics();
    setDevices([]);
    setStats({ total: 0, online: 0, withState: 0, sensors: 0 });
    setMetadataStatus({ connected: false, source: metadataMode });
    setError(null);
    setLoading(true);

    loadInitialDevices();

    const conn = getSharedMqttConnection({ path: REALTIME_PATH, clientIdPrefix: 'rt' });
    mqttConnRef.current = conn;

    const unsubStatus = conn.onStatus(({ connected, status }) => {
      if (!mountedRef.current || !enabledRef.current) return;
      if (connected) {
        markRealtimeMetric('socketOpenMs');
      } else {
        subscriptionAcksRef.current = new Set();
      }
      setStateStatus((prev) => ({
        ...prev,
        connected: Boolean(connected),
        subscribed: Boolean(connected) ? prev.subscribed : false,
      }));
      if (metadataMode === 'ws') {
        setMetadataStatus({ connected: Boolean(connected), source: 'ws' });
        setLoading(false);
      }
      if (!connected && (status === 'error' || status === 'disconnected')) {
        setError(prev => prev || 'Unable to connect to device stream');
      }
      if (connected) {
        setError(prev => (prev && prev.includes('device stream') ? null : prev));
        void loadInitialDevices({ silent: true });
      }
    });

    const unsubs = REALTIME_SUBSCRIPTION_FILTERS.map((filter) => conn.subscribe(filter, handleRealtimeMessage, {
      onSubscribed: ({ filter: subscribedFilter }) => {
        subscriptionAcksRef.current.add(subscribedFilter);
        const subscribed = subscriptionAcksRef.current.size >= REALTIME_SUBSCRIPTION_FILTERS.length;
        if (subscribed) {
          markRealtimeMetric('subscribeCompleteMs');
        }
        setStateStatus((prev) => ({
          ...prev,
          subscribed,
        }));
      },
    }));

    return () => {
      unsubs.forEach(fn => {
        try {
          fn();
        } catch {
          // ignore
        }
      });
      unsubStatus();
      mqttConnRef.current = null;
    };
  }, [enabled, loadInitialDevices, markRealtimeMetric, metadataMode, handleRealtimeMessage, resetRealtimeInitMetrics]);

  const sendDeviceCommand = useCallback((deviceId, statePatch) => new Promise((resolve, reject) => {
    const conn = mqttConnRef.current;
    if (!conn || typeof conn.isConnected !== 'function' || !conn.isConnected()) {
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
      conn.publish(`${COMMAND_PREFIX}${targetId}`, JSON.stringify(envelope), { qos: 0, retained: false });
      resolve();
    } catch (err) {
      reject(err);
    }
  }), []);

  const renameDevice = useCallback(() => Promise.reject(new Error('Rename over MQTT is disabled; use HTTP fallback')), []);

  const commandLockReason = useMemo(() => {
    if (!authReady) return 'Waiting for authentication to finish.';
    if (!stateStatus.connected) return 'Connecting live device channel…';
    if (!stateStatus.subscribed) return 'Waiting for live topic subscriptions…';
    return '';
  }, [authReady, stateStatus.connected, stateStatus.subscribed]);

  const commandsReady = Boolean(authReady && stateStatus.connected && stateStatus.subscribed);

  const connectionInfo = useMemo(() => ({
    metadata: metadataStatus,
    state: stateStatus,
    timings: realtimeMetrics,
    commandsReady,
    commandLockReason,
  }), [commandLockReason, commandsReady, metadataStatus, realtimeMetrics, stateStatus]);

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
