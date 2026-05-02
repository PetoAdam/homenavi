const DEFAULT_COMMAND_LIFECYCLE_TIMEOUT_MS = 45000;
const TERMINAL_COMMAND_STATUSES = new Set(['applied', 'rejected', 'failed', 'timeout']);

function parsePositiveTimeoutMs(value) {
  const parsed = Number.parseInt(String(value ?? '').trim(), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function getRuntimeConfig() {
  if (typeof window === 'undefined') return {};
  return window.__HOMENAVI_RUNTIME_CONFIG__ || {};
}

export function getCommandLifecycleTimeoutMs() {
  const runtimeConfig = getRuntimeConfig();
  return (
    parsePositiveTimeoutMs(runtimeConfig.deviceCommandLifecycleTimeoutMs)
    ?? parsePositiveTimeoutMs(runtimeConfig.deviceCommandTimeoutMs)
    ?? parsePositiveTimeoutMs(runtimeConfig.deviceCommandRequestTimeoutMs)
    ?? parsePositiveTimeoutMs(import.meta.env?.VITE_DEVICE_COMMAND_LIFECYCLE_TIMEOUT_MS)
    ?? parsePositiveTimeoutMs(import.meta.env?.VITE_DEVICE_COMMAND_TIMEOUT_MS)
    ?? parsePositiveTimeoutMs(import.meta.env?.VITE_DEVICE_COMMAND_REQUEST_TIMEOUT_MS)
    ?? DEFAULT_COMMAND_LIFECYCLE_TIMEOUT_MS
  );
}

function normalizeLifecycleStatus(status) {
  const normalized = String(status || '').trim().toLowerCase();
  switch (normalized) {
  case 'accepted':
  case 'queued':
  case 'in_progress':
  case 'applied':
  case 'rejected':
  case 'failed':
  case 'timeout':
    return normalized;
  default:
    return '';
  }
}

export function isTerminalCommandResult(result) {
  if (!result || typeof result !== 'object') return false;
  if (typeof result.terminal === 'boolean') return result.terminal;
  return TERMINAL_COMMAND_STATUSES.has(normalizeLifecycleStatus(result.status));
}

function clonePlainObject(value) {
  if (!value || typeof value !== 'object') return null;
  try {
    // Device state payloads are expected to be plain JSON-like objects.
    return JSON.parse(JSON.stringify(value));
  } catch (err) {
    return null;
  }
}

function stateChangedFromBaseline(baselineState, currentState) {
  if (!baselineState || typeof baselineState !== 'object') return true;
  if (!currentState || typeof currentState !== 'object') return true;

  const keys = new Set([...Object.keys(baselineState), ...Object.keys(currentState)]);
  for (const key of keys) {
    const before = baselineState[key];
    const after = currentState[key];
    if (JSON.stringify(before) !== JSON.stringify(after)) return true;
  }
  return false;
}

function stateSatisfiesExpected(expectedState, currentState) {
  if (!expectedState || typeof expectedState !== 'object') return true;
  if (!currentState || typeof currentState !== 'object') return false;

  return Object.entries(expectedState).every(([key, expectedValue]) => {
    const currentValue = currentState[key];
    return JSON.stringify(currentValue) === JSON.stringify(expectedValue);
  });
}

export function createCommandCorrelationId(payload) {
  return (payload && payload.correlation_id)
    || (typeof crypto !== 'undefined' && crypto.randomUUID
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`);
}

export function withCommandCorrelation(payload, corr) {
  return payload && typeof payload === 'object'
    ? { ...payload, correlation_id: corr }
    : { correlation_id: corr };
}

export function stateVersionFromDevice(device) {
  if (!device) return 0;
  if (device.stateUpdatedAt instanceof Date) {
    return device.stateUpdatedAt.getTime();
  }
  return device.stateUpdatedAt || 0;
}

export function clearPendingTimeout(entry) {
  if (entry?.timeoutId) {
    clearTimeout(entry.timeoutId);
  }
}

export function createPendingCommand(device, payload, options = {}) {
  const corr = createCommandCorrelationId(payload);
  const expectedState = payload && typeof payload === 'object' && payload.state && typeof payload.state === 'object'
    ? payload.state
    : null;
  const timeoutMs = parsePositiveTimeoutMs(options.timeoutMs) ?? getCommandLifecycleTimeoutMs();
  const pending = {
    corr,
    startedAt: Date.now(),
    stateVersion: stateVersionFromDevice(device),
    baselineState: baselineStateFromDevice(device),
    expectedState,
    timeoutId: null,
  };

  if (typeof options.onTimeout === 'function' && timeoutMs > 0) {
    pending.timeoutId = setTimeout(() => {
      options.onTimeout({ corr, pending });
    }, timeoutMs);
  }

  return {
    corr,
    enrichedPayload: withCommandCorrelation(payload, corr),
    pending,
  };
}

export function shouldClearPendingFromDevice(pending, device) {
  if (!pending || !device) return false;
  const currentState = device?.state;
  const currentStateVersion = stateVersionFromDevice(device);
  const pendingStateVersion = Number(pending?.stateVersion || 0);
  const stateCorrMatches = Boolean(device?.lastStateCorr && pending?.corr && device.lastStateCorr === pending.corr);
  const stateVersionAdvanced = currentStateVersion > pendingStateVersion;
  const stateMatchesExpected = stateSatisfiesExpected(pending.expectedState, currentState);
  const stateChanged = stateChangedFromBaseline(pending.baselineState, currentState);

  // State echoes can arrive without a matching terminal command_result. When we observe
  // a newer state (or the matching state correlation) that satisfies the expected patch,
  // treat the pending command as completed so the UI does not stay locked indefinitely.
  if (stateCorrMatches || stateVersionAdvanced) {
    if (stateMatchesExpected) return true;
    if (stateCorrMatches && stateChanged) return true;
    if (!pending?.expectedState && stateChanged) return true;
  }

  const result = device.lastCommandResult;
  const resultMatches = Boolean(result?.corr && pending.corr === result.corr);
  if (!resultMatches) return false;
  if (result?.origin && result.origin !== 'device-hub') return false;
  const status = normalizeLifecycleStatus(result?.status);
  if (!isTerminalCommandResult(result) && !TERMINAL_COMMAND_STATUSES.has(status)) return false;
  if (status !== 'applied') return true;

  if (stateMatchesExpected) return true;
  if (stateChanged) return true;
  return false;
}

export function baselineStateFromDevice(device) {
  return clonePlainObject(device?.state || {}) || {};
}

function readPendingToggleState(state, fallback) {
  if (state && typeof state === 'object') {
    if ('state' in state) return Boolean(state.state);
    if ('on' in state) return Boolean(state.on);
    if ('power' in state) return Boolean(state.power);
  }
  return typeof fallback === 'boolean' ? fallback : null;
}

export function applyPendingStateToDevice(device, pending) {
  if (!device || !pending || !pending.expectedState || typeof pending.expectedState !== 'object') {
    return device;
  }

  const mergedState = {
    ...(device?.state && typeof device.state === 'object' ? device.state : {}),
    ...pending.expectedState,
  };

  return {
    ...device,
    state: mergedState,
    stateHasValues: Object.keys(mergedState).length > 0,
    toggleState: readPendingToggleState(mergedState, device?.toggleState),
    lastStateCorr: pending.corr || device?.lastStateCorr || '',
  };
}
