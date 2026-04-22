// Helpers for managing the auth_token cookie used by api-gateway (and websockets).

const COOKIE_NAME = 'auth_token';
const AUTH_COOKIE_EVENT = 'homenavi:auth-cookie-change';

function shouldUseSecure() {
  try {
    return typeof window !== 'undefined' && window.location && window.location.protocol === 'https:';
  } catch {
    return false;
  }
}

function emitAuthCookieChange(detail) {
  if (typeof window === 'undefined' || typeof window.dispatchEvent !== 'function') {
    return;
  }
  try {
    window.dispatchEvent(new CustomEvent(AUTH_COOKIE_EVENT, { detail }));
  } catch {
    // ignore
  }
}

export function setAuthCookie(token, { maxAgeSeconds = 15 * 60 } = {}) {
  if (!token) {
    clearAuthCookie();
    return;
  }

  const parts = [
    `${COOKIE_NAME}=${encodeURIComponent(token)}`,
    'path=/',
    'SameSite=Strict',
    `max-age=${maxAgeSeconds}`,
  ];

  if (shouldUseSecure()) parts.push('Secure');

  document.cookie = parts.join('; ');
  emitAuthCookieChange({ token, maxAgeSeconds, cleared: false });
}

export function clearAuthCookie() {
  const parts = [
    `${COOKIE_NAME}=`,
    'path=/',
    'SameSite=Strict',
    'max-age=0',
    'expires=Thu, 01 Jan 1970 00:00:00 GMT',
  ];

  if (shouldUseSecure()) parts.push('Secure');

  document.cookie = parts.join('; ');
  emitAuthCookieChange({ token: '', maxAgeSeconds: 0, cleared: true });
}

export function onAuthCookieChange(listener) {
  if (typeof window === 'undefined' || typeof window.addEventListener !== 'function' || typeof listener !== 'function') {
    return () => {};
  }
  const handler = (event) => {
    listener(event?.detail || {});
  };
  window.addEventListener(AUTH_COOKIE_EVENT, handler);
  return () => {
    window.removeEventListener(AUTH_COOKIE_EVENT, handler);
  };
}
