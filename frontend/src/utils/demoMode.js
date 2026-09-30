export const PUBLIC_DEMO_BLOCKED_MESSAGE = 'This action is disabled in the public demo';

function getRuntimeConfig() {
  if (typeof window === 'undefined') return {};
  return window.__HOMENAVI_RUNTIME_CONFIG__ || {};
}

export function isPublicDemoModeEnabled() {
  const runtimeConfig = getRuntimeConfig();
  const value = runtimeConfig.VITE_DEMO_MODE ?? runtimeConfig.demoMode ?? import.meta.env.VITE_DEMO_MODE;
  if (value == null) return false;
  if (typeof value === 'boolean') return value;

  const normalized = String(value).trim().toLowerCase();
  if (!normalized) return false;

  return ['true', '1', 'yes', 'on'].includes(normalized);
}

export function blockPublicDemoAction({ enabled = isPublicDemoModeEnabled(), event, onBlocked, message = PUBLIC_DEMO_BLOCKED_MESSAGE } = {}) {
  if (!enabled) return false;
  event?.preventDefault?.();
  event?.stopPropagation?.();
  onBlocked?.(message);
  return true;
}