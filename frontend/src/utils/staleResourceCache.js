function hasSessionStorage() {
  return typeof window !== 'undefined' && typeof window.sessionStorage !== 'undefined';
}

export function readStaleResourceCache(key, maxAgeMs) {
  if (!key || !hasSessionStorage()) return null;
  try {
    const raw = window.sessionStorage.getItem(key);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return null;
    const ts = Number(parsed.ts);
    if (!Number.isFinite(ts)) return null;
    if (typeof maxAgeMs === 'number' && maxAgeMs > 0 && Date.now() - ts > maxAgeMs) {
      return null;
    }
    return parsed.data ?? null;
  } catch {
    return null;
  }
}

export function writeStaleResourceCache(key, data) {
  if (!key || !hasSessionStorage()) return;
  try {
    window.sessionStorage.setItem(key, JSON.stringify({ ts: Date.now(), data }));
  } catch {
    // Ignore storage quota and serialization failures.
  }
}

export function clearStaleResourceCache(key) {
  if (!key || !hasSessionStorage()) return;
  try {
    window.sessionStorage.removeItem(key);
  } catch {
    // Ignore storage access failures.
  }
}