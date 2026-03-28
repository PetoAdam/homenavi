const TERMINAL_COMMAND_STATUSES = new Set(['applied', 'rejected', 'failed', 'timeout']);

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

export function shouldClearPendingFromDevice(pending, device) {
  if (!pending || !device) return false;
  const result = device.lastCommandResult;
  const resultMatches = Boolean(result?.corr && pending.corr === result.corr);
  if (!resultMatches) return false;
  if (result?.origin && result.origin !== 'device-hub') return false;
  const status = String(result?.status || '').trim().toLowerCase();
  return TERMINAL_COMMAND_STATUSES.has(status);
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
