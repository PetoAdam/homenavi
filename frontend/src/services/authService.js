import http, { setAccessToken as setHttpAccessToken } from './httpClient';
import { REASON_LOGIN_LOCKOUT, REASON_2FA_LOCKOUT, REASON_ADMIN_LOCK } from '../constants/lockoutReasons';

const AUTH_BASE = '/api/auth';

// Auth & user management consolidated service
// Returns normalized { success, data?/tokens?, error? }

export async function login(email, password) {
  const res = await http.post(`${AUTH_BASE}/login/start`, { email, password });
  if (!res.success) {
    // Prioritize explicit lockout (423) before generic 401 invalid credentials
    if (res.status === 423 || /locked/i.test(res.error || '')) {
      const remaining = res.data?.lockout_remaining ?? res.lockout_remaining ?? null;
      const reason = res.data?.reason || REASON_LOGIN_LOCKOUT;
      const unlockAt = res.data?.unlock_at || null;
      let msg = 'Account locked';
      if (reason === REASON_ADMIN_LOCK) {
        msg = 'Account locked by administrator';
      } else if (res.error && !/locked\.\s*try again/i.test(res.error)) {
        // Preserve backend message only if it's not the generic with countdown we replace
        msg = res.error;
      }
      return { success: false, error: msg, lockoutRemaining: remaining, reason, unlockAt };
    }
    if (res.status === 401) {
      return { success: false, error: 'Invalid email or password' };
    }
    return { success: false, error: res.error };
  }
  const d = res.data || {};
  if (d['2fa_required']) {
    return { success: true, twoFA: true, userId: d.user_id, type: d['2fa_type'] };
  }
  if (d.access_token) setHttpAccessToken(d.access_token);
  return { success: true, accessToken: d.access_token, refreshToken: d.refresh_token };
}

export async function finish2FA(userId, code) {
  const res = await http.post(`${AUTH_BASE}/login/finish`, { user_id: userId, code });
  if (!res.success) return { success: false, error: res.error };
  const d = res.data || {};
  if (d.access_token) setHttpAccessToken(d.access_token);
  return { success: true, accessToken: d.access_token, refreshToken: d.refresh_token };
}

export async function signup(firstName, lastName, userName, email, password) {
  const res = await http.post(`${AUTH_BASE}/signup`, { first_name: firstName, last_name: lastName, user_name: userName, email, password });
  if (!res.success) {
    // Surface password policy failures more cleanly
    if (res.status === 400 && /password/i.test(res.error || '')) {
      return { success: false, error: res.error };
    }
    return res; // retain other errors
  }
  return { success: true, user: res.data };
}

export async function refreshToken(refreshToken) {
  const res = await http.post(`${AUTH_BASE}/refresh`, { refresh_token: refreshToken });
  if (!res.success) return { success: false, error: res.error };
  const d = res.data || {};
  if (d.access_token) setHttpAccessToken(d.access_token);
  return { success: true, accessToken: d.access_token, refreshToken: d.refresh_token };
}

export async function logout(refreshToken, accessToken) {
  if (!refreshToken) { setHttpAccessToken(null); return { success: true }; }
  const res = await http.post(`${AUTH_BASE}/logout`, { refresh_token: refreshToken }, { token: accessToken });
  setHttpAccessToken(null);
  return res;
}

export async function getMe(accessToken) {
  if (accessToken) setHttpAccessToken(accessToken);
  const res = await http.get(`${AUTH_BASE}/me`, { token: accessToken });
  if (!res.success) return res;
  return { success: true, user: res.data };
}

// Email verification
export async function requestEmailVerify(userId, accessToken) {
  return await http.post(`${AUTH_BASE}/email/verify/request`, { user_id: userId }, { token: accessToken });
}
export async function confirmEmailVerify(userId, code, accessToken) {
  return await http.post(`${AUTH_BASE}/email/verify/confirm`, { user_id: userId, code }, { token: accessToken });
}

// 2FA flows
export async function request2FAEmail(userId, accessToken) {
  return await http.post(`${AUTH_BASE}/2fa/email/request`, { user_id: userId }, { token: accessToken });
}
export async function verify2FAEmail(userId, code, accessToken) {
  return await http.post(`${AUTH_BASE}/2fa/email/verify`, { user_id: userId, code }, { token: accessToken });
}
export async function setup2FATOTP(userId, accessToken) {
  return await http.post(`${AUTH_BASE}/2fa/setup`, {}, { token: accessToken, params: { user_id: userId } });
}
export async function verify2FATOTP(userId, code, accessToken) {
  return await http.post(`${AUTH_BASE}/2fa/verify`, { user_id: userId, code }, { token: accessToken });
}

// Password management
export async function requestPasswordReset(email) {
  return await http.post(`${AUTH_BASE}/password/reset/request`, { email });
}
export async function confirmPasswordReset(email, code, newPassword) {
  return await http.post(`${AUTH_BASE}/password/reset/confirm`, { email, code, new_password: newPassword });
}
export async function changePassword(currentPassword, newPassword, accessToken) {
  return await http.post(`${AUTH_BASE}/password/change`, { current_password: currentPassword, new_password: newPassword }, { token: accessToken });
}

// Profile picture
export async function generateAvatar(accessToken) {
  return await http.post(`${AUTH_BASE}/profile/generate-avatar`, {}, { token: accessToken });
}
export async function uploadProfilePicture(file, accessToken, userId = null) {
  const form = new FormData();
  form.append('file', file);
  const params = userId ? { user_id: userId } : undefined;
  return await http.post(`${AUTH_BASE}/profile/upload`, form, { token: accessToken, contentType: 'multipart/form-data', params });
}

// OAuth
export function initiateGoogleLogin() {
  window.location.href = `${AUTH_BASE}/oauth/google/login`;
}

// User management (consolidated from former usersService)
export async function listUsers({ q = '', page = 1, pageSize = 20 } = {}, token) {
  return await http.get(`${AUTH_BASE}/users`, { token, params: { q: q || undefined, page, page_size: pageSize } });
}
export async function patchUser(userId, patch, token) {
  return await http.patch(`${AUTH_BASE}/users/${userId}`, patch, { token });
}
export async function lockoutUser(userId, lock, token) {
  return await http.post(`${AUTH_BASE}/users/${userId}/lockout`, { lock }, { token });
}
export async function getUserByEmail(email, token) {
  return await http.get(`${AUTH_BASE}/users`, { token, params: { email } });
}

// We no longer expose deleteUser via auth service (if needed, add endpoint). Placeholder:
export async function deleteUser(userId, token) {
  return await http.post(`${AUTH_BASE}/users/${userId}/delete`, {}, { token }); // adjust server when implemented
}

export default {
  login,
  finish2FA,
  signup,
  refreshToken,
  logout,
  getMe,
  requestEmailVerify,
  confirmEmailVerify,
  request2FAEmail,
  verify2FAEmail,
  setup2FATOTP,
  verify2FATOTP,
  requestPasswordReset,
  confirmPasswordReset,
  changePassword,
  generateAvatar,
  uploadProfilePicture,
  initiateGoogleLogin,
  listUsers,
  patchUser,
  lockoutUser,
  getUserByEmail,
  deleteUser,
};
