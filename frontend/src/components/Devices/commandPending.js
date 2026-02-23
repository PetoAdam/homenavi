export const COMMAND_PENDING_TIMEOUT_MS = 20000;
export const COMMAND_PENDING_NO_EXPECTED_MIN_DELAY_MS = 3000;

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

function normalizeOnOff(value) {
  if (typeof value === 'boolean') return value ? 'on' : 'off';
  if (typeof value === 'number') return value !== 0 ? 'on' : 'off';
  if (typeof value === 'string') {
    const lowered = value.trim().toLowerCase();
    if (['on', 'true', '1', 'yes', 'enabled'].includes(lowered)) return 'on';
    if (['off', 'false', '0', 'no', 'disabled', 'standby'].includes(lowered)) return 'off';
  }
  return null;
}

function expectedPatchSatisfied(expectedState, deviceState) {
  if (!expectedState || typeof expectedState !== 'object') return false;
  if (!deviceState || typeof deviceState !== 'object') return false;

  return Object.entries(expectedState).every(([key, want]) => {
    if (!key) return true;
    const got = deviceState[key];
    if (want === undefined) return true;

    if (typeof want === 'boolean') {
      const gotOnOff = normalizeOnOff(got);
      const wantOnOff = want ? 'on' : 'off';
      return gotOnOff === wantOnOff;
    }
    if (typeof want === 'number') {
      const gotNum = typeof got === 'number' ? got : Number(got);
      return !Number.isNaN(gotNum) && gotNum === want;
    }
    if (typeof want === 'string') {
      const wantNorm = want.trim().toLowerCase();
      const gotNorm = typeof got === 'string'
        ? got.trim().toLowerCase()
        : (normalizeOnOff(got) || String(got ?? '').trim().toLowerCase());
      return gotNorm === wantNorm;
    }
    return JSON.stringify(got) === JSON.stringify(want);
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

export function shouldClearPendingFromDevice(pending, device) {
  if (!pending || !device) return false;
  const result = device.lastCommandResult;
  if (!result) return false;
  if (!pending.corr || !result.corr || pending.corr !== result.corr) return false;

  if (!result.success) return true;

  const minDelaySatisfied = typeof pending.startedAt === 'number'
    ? (Date.now() - pending.startedAt) >= COMMAND_PENDING_NO_EXPECTED_MIN_DELAY_MS
    : true;

  // Prefer clearing when the device state actually reflects what we asked for.
  // This prevents "snap back" for cloud devices where a refresh can arrive with the old value.
  if (pending.expectedState && expectedPatchSatisfied(pending.expectedState, device.state || {})) {
    // Delay clearing slightly even when expected is satisfied, because an optimistic publish
    // can satisfy this immediately and then a stale refresh can arrive right after.
    return minDelaySatisfied;
  }

  const stateTs = stateVersionFromDevice(device);
  const baselineTs = pending.stateVersion || 0;
  const resultTs = Number(result.ts || 0);
  const hasStateAdvanced = stateTs && stateTs > baselineTs;
  const stateCoversResult = stateTs && resultTs && stateTs >= resultTs;
  const noExpectedMinDelaySatisfied = minDelaySatisfied;
  const baselineState = pending.baselineState;
  const stateChanged = stateChangedFromBaseline(baselineState, device.state || {});

  // Fallback for legacy integrations/devices where we don't have an expected patch.
  // Require a minimum delay and an actual change from the baseline state to avoid
  // clearing on a refresh/realtime event that re-emits the old value.
  return noExpectedMinDelaySatisfied && stateChanged && (hasStateAdvanced || stateCoversResult);
}

export function baselineStateFromDevice(device) {
  return clonePlainObject(device?.state || {}) || {};
}
