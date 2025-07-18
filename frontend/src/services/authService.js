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
    console.error('Login error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Login failed';
    return { success: false, error: errorMessage };
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
    console.error('2FA error:', err);
    console.error('2FA error response:', err.response?.data);
    let errorMessage = '2FA verification failed';
    
    if (err.response?.data) {
      if (typeof err.response.data === 'string') {
        errorMessage = err.response.data;
      } else if (err.response.data.message) {
        errorMessage = err.response.data.message;
      } else if (err.response.data.error) {
        errorMessage = err.response.data.error;
      }
    } else if (err.message) {
      errorMessage = err.message;
    }
    
    return { success: false, error: errorMessage };
  }
}

export async function signup(firstName, lastName, userName, email, password) {
  try {
    const resp = await axios.post(`${API_URL}/signup`, {
      first_name: firstName,
      last_name: lastName,
      user_name: userName,
      email,
      password
    });
    return { success: true, user: resp.data };
  } catch (err) {
    console.error('Signup error:', err);
    const errorMessage = err.response?.data?.message || err.response?.data?.error || err.message || 'Signup failed';
    return { success: false, error: errorMessage };
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
    console.error('Refresh token error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Refresh failed';
    return { success: false, error: errorMessage };
  }
}

export async function logout(refreshToken, accessToken) {
  try {
    // Only call logout endpoint if we have a refresh token
    if (refreshToken) {
      const headers = accessToken ? { Authorization: `Bearer ${accessToken}` } : {};
      await axios.post(`${API_URL}/logout`, { refresh_token: refreshToken }, { headers });
    }
    return { success: true };
  } catch (err) {
    console.error('Logout error:', err);
    // Even if logout fails, we still want to clear local storage
    return { success: true };
  }
}

export async function getMe(accessToken) {
  try {
    const resp = await axios.get(`${API_URL}/me`, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true, user: resp.data };
  } catch (err) {
    console.error('GetMe error:', err);
    return { success: false, error: err.response?.data?.error || err.response?.data?.message || err.message || 'Failed to fetch user' };
  }
}

export async function requestEmailVerify(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/email/verify/request`, { user_id: userId }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    console.error('Email verify request error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Email verify request failed';
    return { success: false, error: errorMessage };
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
    console.error('Email verification error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Email verification failed';
    return { success: false, error: errorMessage };
  }
}

export async function request2FAEmail(userId, accessToken) {
  try {
    const headers = accessToken ? { Authorization: `Bearer ${accessToken}` } : {};
    await axios.post(`${API_URL}/2fa/email/request`, { user_id: userId }, { headers });
    return { success: true };
  } catch (err) {
    console.error('2FA email request error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || '2FA email request failed';
    return { success: false, error: errorMessage };
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
    console.error('2FA verification error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || '2FA verification failed';
    return { success: false, error: errorMessage };
  }
}

export async function setup2FATOTP(userId, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/2fa/setup?user_id=${userId}`, {}, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true, secret: resp.data.secret, otpauthUrl: resp.data.otpauth_url };
  } catch (err) {
    return { success: false, error: err.response?.data?.error || err.response?.data?.message || err.message || '2FA TOTP setup failed' };
  }
}

export async function verify2FATOTP(userId, code, accessToken) {
  try {
    await axios.post(`${API_URL}/2fa/verify`, { user_id: userId, code }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    return { success: false, error: err.response?.data?.error || err.response?.data?.message || err.message || '2FA TOTP verify failed' };
  }
}

export async function patchUser(userId, patch, accessToken) {
  try {
    await axios.patch(`/api/users/${userId}`, patch, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    console.error('Patch user error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Patch user failed';
    return { success: false, error: errorMessage };
  }
}

export async function deleteUser(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/delete`, { user_id: userId }, { headers: { Authorization: `Bearer ${accessToken}` } });
    return { success: true };
  } catch (err) {
    console.error('Delete user error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Delete user failed';
    return { success: false, error: errorMessage };
  }
}

export async function requestPasswordReset(email) {
  try {
    const resp = await axios.post(`${API_URL}/password/reset/request`, { email });
    return { success: true, data: resp.data };
  } catch (err) {
    console.error('Password reset request error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Failed to send reset code';
    return { success: false, error: errorMessage };
  }
}

export async function confirmPasswordReset(email, code, newPassword) {
  try {
    const resp = await axios.post(`${API_URL}/password/reset/confirm`, {
      email,
      code,
      new_password: newPassword
    });
    return { success: true, data: resp.data };
  } catch (err) {
    console.error('Password reset confirm error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Password reset failed';
    return { success: false, error: errorMessage };
  }
}

export async function changePassword(currentPassword, newPassword, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/password/change`, 
      { current_password: currentPassword, new_password: newPassword },
      {
        headers: {
          'Authorization': `Bearer ${accessToken}`,
          'Content-Type': 'application/json'
        }
      }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    console.error('Password change error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Password change failed';
    return { success: false, error: errorMessage };
  }
}

// Profile picture service functions
export const generateAvatar = async (accessToken) => {
  try {
    const resp = await axios.post('/api/auth/profile/generate-avatar', {}, {
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json'
      }
    });
    return { success: true, data: resp.data };
  } catch (err) {
    console.error('Avatar generation error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Avatar generation failed';
    return { success: false, error: errorMessage };
  }
};

export const uploadProfilePicture = async (file, accessToken, userId = null) => {
  try {
    const formData = new FormData();
    formData.append('file', file);
    
    let url = '/api/auth/profile/upload';
    if (userId) {
      url += `?user_id=${userId}`;
    }
    
    const resp = await axios.post(url, formData, {
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'multipart/form-data'
      }
    });
    return { success: true, data: resp.data };
  } catch (err) {
    console.error('Upload profile picture error:', err);
    const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || 'Upload failed';
    return { success: false, error: errorMessage };
  }
};
