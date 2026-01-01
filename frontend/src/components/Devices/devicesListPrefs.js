const STORAGE_KEY = 'homenavi.devices.list.prefs.v1';

function safeString(value) {
  return typeof value === 'string' ? value : '';
}

function safeBoolean(value, fallback) {
  if (typeof value === 'boolean') return value;
  if (value === 'true') return true;
  if (value === 'false') return false;
  return fallback;
}

export function loadDevicesListPrefs() {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return {};
    return parsed;
  } catch {
    return {};
  }
}

export function normalizeDevicesListPrefs(raw) {
  const src = raw && typeof raw === 'object' ? raw : {};
  return {
    metadataMode: ['ws', 'rest'].includes(safeString(src.metadataMode)) ? src.metadataMode : 'ws',
    groupByRoom: safeBoolean(src.groupByRoom, true),
    protocolFilter: safeString(src.protocolFilter) || 'all',
    roomFilter: safeString(src.roomFilter) || 'all',
    tagFilter: safeString(src.tagFilter) || 'all',
    searchTerm: safeString(src.searchTerm),
  };
}

export function saveDevicesListPrefs(prefs) {
  try {
    const normalized = normalizeDevicesListPrefs(prefs);
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(normalized));
  } catch {
    // ignore
  }
}
