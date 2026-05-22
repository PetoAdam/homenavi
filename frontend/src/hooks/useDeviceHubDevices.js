import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { getSharedMqttConnection } from '../services/realtime/sharedMqtt';
import { clearStaleResourceCache, readStaleResourceCache, writeStaleResourceCache } from '../utils/staleResourceCache';

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
const DEVICE_LIST_CONNECT_REFRESH_FRESHNESS_MS = 3000;
const DEVICE_LIST_CACHE_TTL_MS = 20 * 1000;

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

function normalizeManagementActions(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map(item => {
      if (!item || typeof item !== 'object') return null;
      const id = `${item.id || ''}`.trim();
      const command = `${item.command || ''}`.trim().toLowerCase();
      const mode = `${item.mode || ''}`.trim().toLowerCase();
      const label = `${item.label || ''}`.trim();
      if (!id || !command || !label) return null;
      return {
        id,
        command,
        mode,
        label,
        description: normalizeDescription(item.description),
      };
    })
    .filter(Boolean);
}

export function normalizeDeviceConfiguration(value, capabilitiesValue = [], inputsValue = [], protocolValue = '') {
  const capabilities = ensureArray(capabilitiesValue);
  const inputs = ensureArray(inputsValue);
  const protocol = `${protocolValue || ''}`.trim().toLowerCase();
  const fallbackReady = capabilities.length > 0 || inputs.length > 0;
  const fallback = fallbackReady
    ? { ready: true, status: 'configured', message: '' }
    : {
        ready: false,
        status: 'incomplete',
        message: protocol === 'zigbee'
          ? 'Device metadata is incomplete. Refresh or reinterview the device to load capabilities and controls.'
          : 'Device metadata is incomplete. The adapter has not reported capabilities or controls yet.',
      };

  if (!value || typeof value !== 'object') {
    return fallback;
  }

  const ready = typeof value.ready === 'boolean' ? value.ready : fallback.ready;
  const status = `${value.status || (ready ? 'configured' : 'incomplete')}`.trim().toLowerCase() || (ready ? 'configured' : 'incomplete');
  const message = normalizeDescription(value.message) || (ready ? '' : fallback.message);
  return { ready, status, message };
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

export function normalizeTimestampMs(value) {
  if (value instanceof Date) {
    const timestamp = value.getTime();
    return Number.isNaN(timestamp) ? null : timestamp;
  }
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return value;
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return null;
    const numeric = Number(trimmed);
    if (Number.isFinite(numeric) && numeric > 0) {
      return numeric;
    }
    const parsed = Date.parse(trimmed);
    return Number.isNaN(parsed) ? null : parsed;
  }
  return null;
}

export function resolveRestStateUpdatedAt(raw, prev = null) {
  const explicit = normalizeTimestampMs(
    raw?.state_updated_at
    ?? raw?.stateUpdatedAt
    ?? raw?.state_updated
    ?? raw?.stateUpdated,
  );
  if (explicit) return explicit;
  const previous = normalizeTimestampMs(prev?.stateUpdatedAt);
  return previous || null;
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

function normalizePairingAddedDevices(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map(item => {
      if (!item || typeof item !== 'object') return null;
      const addedAt = item.added_at || item.addedAt;
      const normalizedAddedAt = addedAt ? new Date(addedAt) : null;
      const updatedAt = item.updated_at || item.updatedAt;
      const normalizedUpdatedAt = updatedAt ? new Date(updatedAt) : null;
      return {
        deviceId: item.device_id || item.deviceId || '',
        protocol: item.protocol || '',
        externalId: item.external_id || item.externalId || '',
        name: item.name || '',
        state: item.state || '',
        type: item.type || '',
        manufacturer: item.manufacturer || '',
        model: item.model || '',
        description: normalizeDescription(item.description),
        icon: item.icon || '',
        addedAt: normalizedAddedAt && !Number.isNaN(normalizedAddedAt.getTime()) ? normalizedAddedAt : null,
        updatedAt: normalizedUpdatedAt && !Number.isNaN(normalizedUpdatedAt.getTime()) ? normalizedUpdatedAt : null,
      };
    })
    .filter(item => item && (item.deviceId || item.externalId));
}

function pairingAddedDeviceStateRank(value) {
  switch (`${value || ''}`.trim().toLowerCase()) {
    case 'detected':
      return 10;
    case 'finalizing':
      return 20;
    case 'completed':
      return 30;
    case 'failed':
      return 40;
    default:
      return 0;
  }
}

function pairingAddedDeviceMatches(left, right) {
  if (!left || !right) return false;
  const leftDeviceId = `${left.deviceId || ''}`.trim().toLowerCase();
  const rightDeviceId = `${right.deviceId || ''}`.trim().toLowerCase();
  if (leftDeviceId && rightDeviceId && leftDeviceId === rightDeviceId) {
    return true;
  }
  const leftExternalId = `${left.externalId || ''}`.trim().toLowerCase();
  const rightExternalId = `${right.externalId || ''}`.trim().toLowerCase();
  return Boolean(leftExternalId && rightExternalId && leftExternalId === rightExternalId);
}

function mergePairingAddedDevice(existing, incoming) {
  const addedAtCandidates = [existing?.addedAt, incoming?.addedAt]
    .filter(value => value instanceof Date && !Number.isNaN(value.getTime()));
  const updatedAtCandidates = [existing?.updatedAt, incoming?.updatedAt]
    .filter(value => value instanceof Date && !Number.isNaN(value.getTime()));
  const existingStateRank = pairingAddedDeviceStateRank(existing?.state);
  const incomingStateRank = pairingAddedDeviceStateRank(incoming?.state);
  return {
    ...existing,
    ...incoming,
    deviceId: incoming?.deviceId || existing?.deviceId || '',
    protocol: incoming?.protocol || existing?.protocol || '',
    externalId: incoming?.externalId || existing?.externalId || '',
    name: incoming?.name || existing?.name || '',
    state: incomingStateRank >= existingStateRank ? (incoming?.state || existing?.state || '') : (existing?.state || incoming?.state || ''),
    type: incoming?.type || existing?.type || '',
    manufacturer: incoming?.manufacturer || existing?.manufacturer || '',
    model: incoming?.model || existing?.model || '',
    description: incoming?.description || existing?.description || '',
    icon: incoming?.icon || existing?.icon || '',
    addedAt: addedAtCandidates.length > 0
      ? new Date(Math.min(...addedAtCandidates.map(value => value.getTime())))
      : null,
    updatedAt: updatedAtCandidates.length > 0
      ? new Date(Math.max(...updatedAtCandidates.map(value => value.getTime())))
      : null,
  };
}

export function mergePairingAddedDevices(existingValue, incomingValue) {
  const existing = normalizePairingAddedDevices(existingValue);
  const incoming = normalizePairingAddedDevices(incomingValue);
  if (existing.length === 0) return incoming;
  if (incoming.length === 0) return existing;

  const merged = [...existing];
  incoming.forEach(item => {
    const index = merged.findIndex(candidate => pairingAddedDeviceMatches(candidate, item));
    if (index >= 0) {
      merged[index] = mergePairingAddedDevice(merged[index], item);
      return;
    }
    merged.push(item);
  });
  return merged;
}

export function shouldSkipFreshDeviceListFetch(lastSuccessfulLoadAt, now = Date.now(), freshnessMs = DEVICE_LIST_CONNECT_REFRESH_FRESHNESS_MS) {
  if (!Number.isFinite(lastSuccessfulLoadAt) || lastSuccessfulLoadAt <= 0) {
    return false;
  }
  return (now - lastSuccessfulLoadAt) < freshnessMs;
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
  const stage = canonicalizePairingProgressValue(raw.stage || '');
  return {
    id: raw.id || `${protocol}-${raw.started_at || Date.now()}`,
    protocol,
    status: normalizePairingRuntimeStatus(raw.status, stage),
    active: Boolean(raw.active),
    stage,
    mode: raw.mode || '',
    flowId: raw.flow_id || raw.flowId || '',
    message: raw.message || '',
    errorCode: raw.error_code || raw.errorCode || '',
    requiredInputs: Array.isArray(raw.required_inputs) ? raw.required_inputs : [],
    inputs: raw.inputs && typeof raw.inputs === 'object' ? { ...raw.inputs } : {},
    startedAt: normalizeDate(raw.started_at),
    expiresAt: normalizeDate(raw.expires_at),
    deviceId: raw.device_id || raw.deviceId || '',
    allowMultipleDevices: Boolean(raw.allow_multiple_devices || raw.allowMultipleDevices),
    addedDevices: normalizePairingAddedDevices(raw.added_devices || raw.addedDevices),
    metadata: raw.metadata && typeof raw.metadata === 'object' ? { ...raw.metadata } : {},
    progress: raw,
  };
}

function isTerminalPairingStatus(status) {
  return ['completed', 'failed', 'timeout', 'stopped', 'error'].includes(`${status || ''}`.toLowerCase());
}

const PAIRING_PROGRESS_ALIASES = {
  device_announce: 'device_detected',
  device_announced: 'device_detected',
  interview_started: 'interviewing',
  interview_succeeded: 'interview_complete',
};

const PAIRING_PROGRESS_RANK = {
  starting: 10,
  active: 20,
  in_progress: 20,
  device_joined: 30,
  device_detected: 30,
  interviewing: 40,
  interview_complete: 50,
  needs_input: 60,
  completed: 100,
  failed: 100,
  timeout: 100,
  stopped: 100,
  error: 100,
};

function canonicalizePairingProgressValue(value) {
  const normalized = `${value || ''}`.trim().toLowerCase();
  if (!normalized) return '';
  return PAIRING_PROGRESS_ALIASES[normalized] || normalized;
}

function pairingProgressRank(stage, status) {
  const normalizedStage = canonicalizePairingProgressValue(stage);
  const normalizedStatus = canonicalizePairingProgressValue(status);
  return Math.max(
    PAIRING_PROGRESS_RANK[normalizedStage] || 0,
    PAIRING_PROGRESS_RANK[normalizedStatus] || 0,
  );
}

function normalizePairingRuntimeStatus(status, stage) {
  const normalizedStatus = canonicalizePairingProgressValue(status);
  const normalizedStage = canonicalizePairingProgressValue(stage);

  if (isTerminalPairingStatus(normalizedStage)) {
    return normalizedStage;
  }
  if (isTerminalPairingStatus(normalizedStatus)) {
    return normalizedStatus;
  }
  if (normalizedStage === 'interview_complete' || normalizedStage === 'interviewing' || normalizedStage === 'device_detected' || normalizedStage === 'device_joined') {
    return normalizedStage;
  }
  return normalizedStatus || normalizedStage || 'in_progress';
}

export function buildPairingProgressSession(data, protocol, existing = null) {
  const normalizedProtocol = `${protocol || ''}`.trim().toLowerCase();
  if (!normalizedProtocol) return null;
  const explicitId = typeof data?.id === 'string' ? data.id : '';
  const stage = canonicalizePairingProgressValue(data?.stage || '');
  const status = normalizePairingRuntimeStatus(data?.status, stage);
  const isTerminal = isTerminalPairingStatus(status) || isTerminalPairingStatus(stage);
  const metadata = data?.metadata && typeof data.metadata === 'object' ? { ...data.metadata } : null;
  const existingStage = canonicalizePairingProgressValue(existing?.stage || '');
  const existingStatus = normalizePairingRuntimeStatus(existing?.status, existingStage);
  const shouldPreserveProgress = Boolean(existing?.active)
    && !isTerminal
    && pairingProgressRank(stage, status) > 0
    && pairingProgressRank(stage, status) < pairingProgressRank(existingStage, existingStatus);

  const finalStage = shouldPreserveProgress ? existingStage || stage : stage;
  const finalStatus = shouldPreserveProgress ? existingStatus || status : status;
  const finalIsTerminal = isTerminalPairingStatus(finalStatus) || isTerminalPairingStatus(finalStage);

  return {
    id: explicitId || existing?.id || normalizedProtocol,
    protocol: normalizedProtocol,
    status: finalStatus,
    active: typeof data?.active === 'boolean' ? (shouldPreserveProgress ? existing?.active : data.active) : !finalIsTerminal,
    allowMultipleDevices: Boolean(data?.allow_multiple_devices || data?.allowMultipleDevices || existing?.allowMultipleDevices),
    addedDevices: mergePairingAddedDevices(existing?.addedDevices || [], data?.added_devices || data?.addedDevices),
    metadata: metadata || existing?.metadata || {},
    stage: finalStage,
    mode: data?.mode || existing?.mode || '',
    flowId: data?.flow_id || data?.flowId || existing?.flowId || '',
    message: data?.message || '',
    errorCode: data?.error_code || data?.errorCode || '',
    requiredInputs: Array.isArray(data?.required_inputs) ? data.required_inputs : (existing?.requiredInputs || []),
    deviceId: data?.device_id || data?.deviceId || existing?.deviceId || '',
    progress: data,
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
  const configuration = normalizeDeviceConfiguration(raw.configuration, capabilities, inputs, raw.protocol || '');
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
    configuration,
    managementActions: normalizeManagementActions(raw.management_actions || raw.managementActions),
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
    configuration: normalizeDeviceConfiguration(
      data?.configuration ?? prev?.configuration,
      data?.capabilities ?? prev?.capabilities,
      data?.inputs ?? prev?.inputs,
      data?.protocol ?? prev?.protocol ?? protocolHint,
    ),
    managementActions: normalizeManagementActions(data?.management_actions ?? data?.managementActions ?? prev?.managementActions),
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
  const deviceListCacheKey = accessToken ? `homenavi:device-hub:list:${accessToken.slice(-16)}` : 'homenavi:device-hub:list:anon';

  const devicesRef = useRef(new Map());
  const mqttConnRef = useRef(null);
  const updateScheduledRef = useRef(false);
  const mountedRef = useRef(false);
  const enabledRef = useRef(enabled);
  const realtimeInitStartedAtRef = useRef(0);
  const subscriptionAcksRef = useRef(new Set());
  const deviceLoadPromiseRef = useRef(null);
  const lastSuccessfulDeviceLoadAtRef = useRef(0);

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
    if (realtimeMetrics.authReadyMs != null) return;
    markRealtimeMetric('authReadyMs');
  }, [authReady, enabled, markRealtimeMetric, realtimeMetrics.authReadyMs]);

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

  const hydrateDeviceList = useCallback((payload, { markFresh = false } = {}) => {
    if (!Array.isArray(payload)) {
      return false;
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

      if (String(mapKey).startsWith('zigbee/') && !isCanonicalZigbeeId(mapKey)) {
        return;
      }

      const stateObj = ensureStateObject(item.state);
      const prev = previousEntries.get(mapKey) || {};
      const stateUpdatedAt = Object.keys(stateObj).length > 0
        ? resolveRestStateUpdatedAt(item, prev)
        : (prev.stateUpdatedAt ?? null);
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
        configuration: normalizeDeviceConfiguration(item.configuration, item.capabilities, item.inputs, item.protocol),
        description: normalizeDescription(item.description),
        _last_state: Object.keys(stateObj).length > 0 ? stateObj : ensureStateObject(prev._last_state),
        metadataUpdatedAt: now,
        stateUpdatedAt,
        __hasMetadata: true,
      });
    });
    if (!mountedRef.current || !enabledRef.current) {
      return false;
    }
    devicesRef.current = next;
    if (markFresh) {
      lastSuccessfulDeviceLoadAtRef.current = Date.now();
    }
    schedulePublish();
    return true;
  }, [schedulePublish]);

  const loadInitialDevices = useCallback(async ({ silent = false, minFreshMs = 0 } = {}) => {
    if (minFreshMs > 0 && shouldSkipFreshDeviceListFetch(lastSuccessfulDeviceLoadAtRef.current, Date.now(), minFreshMs)) {
      return true;
    }
    if (deviceLoadPromiseRef.current) {
      return deviceLoadPromiseRef.current;
    }
    const request = (async () => {
    try {
      const response = await fetch('/api/hdp/devices', buildFetchOptions());
      if (!response.ok) {
        throw new Error(`request failed with status ${response.status}`);
      }
      const payload = await response.json();
      if (!Array.isArray(payload)) {
        throw new Error('invalid device list payload');
      }
      if (!hydrateDeviceList(payload, { markFresh: true })) {
        return false;
      }
      if (metadataMode === 'rest') {
        setMetadataStatus({ connected: true, source: 'rest' });
      }
      writeStaleResourceCache(deviceListCacheKey, payload);
      setError(prev => {
        if (!prev) return prev;
        if (prev.includes('device list') || prev.includes('device stream')) return null;
        return prev;
      });
      schedulePublish();
      return true;
    } catch (err) {
      console.warn('Device list fetch failed', err);
      if (!mountedRef.current || !enabledRef.current) {
        return false;
      }
      if (!silent && metadataMode === 'rest') {
        setMetadataStatus({ connected: false, source: 'rest' });
        setError(prev => prev || 'Unable to load device list');
        setLoading(false);
      }
      return false;
    } finally {
      if (deviceLoadPromiseRef.current === request) {
        deviceLoadPromiseRef.current = null;
      }
    }
    })();
    deviceLoadPromiseRef.current = request;
    return request;
  }, [buildFetchOptions, deviceListCacheKey, hydrateDeviceList, metadataMode]);

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
      setPairingSessions(prev => {
        const existing = prev?.[protocol];
        const session = buildPairingProgressSession(data, protocol, existing);
        if (!session) {
          return prev;
        }
        const explicitSessionId = typeof data.id === 'string' ? data.id : '';
        if (existing?.active && !session.active && explicitSessionId && existing.id && explicitSessionId !== existing.id) {
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
      deviceLoadPromiseRef.current = null;
      lastSuccessfulDeviceLoadAtRef.current = 0;
      clearStaleResourceCache(deviceListCacheKey);
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
    if (authReady) {
      markRealtimeMetric('authReadyMs');
    }
    setDevices([]);
    setStats({ total: 0, online: 0, withState: 0, sensors: 0 });
    setMetadataStatus({ connected: false, source: metadataMode });
    setError(null);
    setLoading(true);

    const cachedDevices = readStaleResourceCache(deviceListCacheKey, DEVICE_LIST_CACHE_TTL_MS);
    if (hydrateDeviceList(cachedDevices)) {
      setLoading(false);
    }

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
        void loadInitialDevices({ silent: true, minFreshMs: DEVICE_LIST_CONNECT_REFRESH_FRESHNESS_MS });
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
  }, [authReady, deviceListCacheKey, enabled, handleRealtimeMessage, hydrateDeviceList, loadInitialDevices, markRealtimeMetric, metadataMode, resetRealtimeInitMetrics]);

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
