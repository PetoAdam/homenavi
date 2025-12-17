// Helpers for managing the auth_token cookie used by api-gateway (and websockets).

const COOKIE_NAME = 'auth_token';

function shouldUseSecure() {
  try {
    return typeof window !== 'undefined' && window.location && window.location.protocol === 'https:';
  } catch {
    return false;
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
}
