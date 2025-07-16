// Professional auth service for real backend integration
import axios from 'axios';

const API_URL = '/api/auth'; // Use gateway path

export async function login(email, password, twoFACode = null) {
  try {
    // Step 1: login start
    const resp = await axios.post(`${API_URL}/login/start`, { email, password });
    if (resp.data && resp.data["2fa_required"]) {
      // 2FA required
      return { twoFA: true, userId: resp.data.user_id, type: resp.data["2fa_type"] };
    }
    // Success: tokens
    return {
      success: true,
      accessToken: resp.data.access_token,
      refreshToken: resp.data.refresh_token,
    };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Login failed' };
  }
}

export async function finish2FA(userId, code) {
  try {
    const resp = await axios.post(`${API_URL}/login/finish`, { user_id: userId, code });
    return {
      success: true,
      accessToken: resp.data.access_token,
      refreshToken: resp.data.refresh_token,
    };
  } catch (err) {
    return { success: false, error: err.response?.data || '2FA failed' };
  }
}

export async function signup(userName, email, password) {
  try {
    const resp = await axios.post(`${API_URL}/signup`, { user_name: userName, email, password });
    return { success: true, user: resp.data };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Signup failed' };
  }
}

export async function refreshToken(refreshToken) {
  try {
    const resp = await axios.post(`${API_URL}/refresh`, { refresh_token: refreshToken });
    return {
      success: true,
      accessToken: resp.data.access_token,
      refreshToken: resp.data.refresh_token,
    };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Refresh failed' };
  }
}

export async function logout(refreshToken) {
  try {
    await axios.post(`${API_URL}/logout`, { refresh_token: refreshToken });
    return { success: true };
  } catch (err) {
    return { success: false };
  }
}

export async function getMe(accessToken) {
  try {
    const resp = await axios.get(`${API_URL}/me`, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true, user: resp.data };
  } catch (err) {
    return { success: false };
  }
}

export async function requestEmailVerify(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/email/verify/request`, { user_id: userId }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Email verify request failed' };
  }
}

export async function confirmEmailVerify(userId, code, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/email/verify/confirm`, 
      { user_id: userId, code },
      {
        headers: {
          'Authorization': `Bearer ${accessToken}`,
          'Content-Type': 'application/json'
        }
      }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Email verification failed' };
  }
}

export async function request2FAEmail(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/2fa/email/request`, { user_id: userId }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || '2FA email request failed' };
  }
}

export async function verify2FAEmail(userId, code, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/2fa/email/verify`, 
      { user_id: userId, code },
      {
        headers: {
          'Authorization': `Bearer ${accessToken}`,
          'Content-Type': 'application/json'
        }
      }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    return { success: false, error: err.response?.data || '2FA verification failed' };
  }
}

export async function setup2FATOTP(userId, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/2fa/setup?user_id=${userId}`, {}, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true, secret: resp.data.secret, otpauthUrl: resp.data.otpauth_url };
  } catch (err) {
    return { success: false, error: err.response?.data || '2FA TOTP setup failed' };
  }
}

export async function verify2FATOTP(userId, code, accessToken) {
  try {
    await axios.post(`${API_URL}/2fa/verify`, { user_id: userId, code }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || '2FA TOTP verify failed' };
  }
}

export async function patchUser(userId, patch, accessToken) {
  try {
    await axios.patch(`/api/users/${userId}`, patch, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Patch user failed' };
  }
}

export async function deleteUser(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/delete`, { user_id: userId }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Delete user failed' };
  }
}

export async function requestPasswordReset(email) {
  try {
    await axios.post(`${API_URL}/password/reset/request`, { email });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Password reset request failed' };
  }
}

export async function confirmPasswordReset(email, code, newPassword) {
  try {
    await axios.post(`${API_URL}/password/reset/confirm`, { email, code, new_password: newPassword });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Password reset failed' };
  }
}

// ...add more if needed
