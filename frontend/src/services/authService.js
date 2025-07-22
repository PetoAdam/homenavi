// Professional auth service for real backend integration
import axios from 'axios';

const API_URL = '/api/auth'; // Use gateway path
const API_USERS_URL = '/api/users'; // Users API path

// Helper function to create auth headers
const createAuthHeaders = (accessToken, contentType = 'application/json') => ({
  'Authorization': `Bearer ${accessToken}`,
  'Content-Type': contentType
});

// Helper function to handle API errors consistently
const handleError = (err, defaultMessage) => {
  console.error(`${defaultMessage}:`, err);
  const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || defaultMessage;
  return { success: false, error: errorMessage };
};

export async function login(email, password, twoFACode = null) {
  try {
    const resp = await axios.post(`${API_URL}/login/start`, { email, password });
    if (resp.data && resp.data["2fa_required"]) {
      return { twoFA: true, userId: resp.data.user_id, type: resp.data["2fa_type"] };
    }
    return {
      success: true,
      accessToken: resp.data.access_token,
      refreshToken: resp.data.refresh_token,
    };
  } catch (err) {
    return handleError(err, 'Login failed');
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
    return handleError(err, '2FA verification failed');
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
    return handleError(err, 'Signup failed');
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
    return handleError(err, 'Refresh failed');
  }
}

export async function logout(refreshToken, accessToken) {
  try {
    // Only call logout endpoint if we have a refresh token
    if (refreshToken) {
      const headers = accessToken ? createAuthHeaders(accessToken) : {};
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
    const resp = await axios.get(`${API_URL}/me`, { headers: createAuthHeaders(accessToken) });
    return { success: true, user: resp.data };
  } catch (err) {
    return handleError(err, 'Failed to fetch user');
  }
}

export async function requestEmailVerify(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/email/verify/request`, { user_id: userId }, { headers: createAuthHeaders(accessToken) });
    return { success: true };
  } catch (err) {
    return handleError(err, 'Email verify request failed');
  }
}

export async function confirmEmailVerify(userId, code, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/email/verify/confirm`, 
      { user_id: userId, code },
      { headers: createAuthHeaders(accessToken) }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Email verification failed');
  }
}

export async function request2FAEmail(userId, accessToken) {
  try {
    const headers = accessToken ? createAuthHeaders(accessToken) : {};
    await axios.post(`${API_URL}/2fa/email/request`, { user_id: userId }, { headers });
    return { success: true };
  } catch (err) {
    return handleError(err, '2FA email request failed');
  }
}

export async function verify2FAEmail(userId, code, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/2fa/email/verify`, 
      { user_id: userId, code },
      { headers: createAuthHeaders(accessToken) }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, '2FA verification failed');
  }
}

export async function setup2FATOTP(userId, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/2fa/setup?user_id=${userId}`, {}, { headers: createAuthHeaders(accessToken) });
    return { success: true, secret: resp.data.secret, otpauthUrl: resp.data.otpauth_url };
  } catch (err) {
    return handleError(err, '2FA TOTP setup failed');
  }
}

export async function verify2FATOTP(userId, code, accessToken) {
  try {
    await axios.post(`${API_URL}/2fa/verify`, { user_id: userId, code }, { headers: createAuthHeaders(accessToken) });
    return { success: true };
  } catch (err) {
    return handleError(err, '2FA TOTP verify failed');
  }
}

export async function patchUser(userId, patch, accessToken) {
  try {
    await axios.patch(`${API_USERS_URL}/${userId}`, patch, { headers: createAuthHeaders(accessToken) });
    return { success: true };
  } catch (err) {
    return handleError(err, 'Patch user failed');
  }
}

export async function deleteUser(userId, accessToken) {
  try {
    await axios.post(`${API_URL}/delete`, { user_id: userId }, { headers: createAuthHeaders(accessToken) });
    return { success: true };
  } catch (err) {
    return handleError(err, 'Delete user failed');
  }
}

export async function requestPasswordReset(email) {
  try {
    const resp = await axios.post(`${API_URL}/password/reset/request`, { email });
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Failed to send reset code');
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
    return handleError(err, 'Password reset failed');
  }
}

export async function changePassword(currentPassword, newPassword, accessToken) {
  try {
    const resp = await axios.post(`${API_URL}/password/change`, 
      { current_password: currentPassword, new_password: newPassword },
      { headers: createAuthHeaders(accessToken) }
    );
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Password change failed');
  }
}

// Profile picture service functions
export const generateAvatar = async (accessToken) => {
  try {
    const resp = await axios.post(`${API_URL}/profile/generate-avatar`, {}, {
      headers: createAuthHeaders(accessToken)
    });
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Avatar generation failed');
  }
};

export const uploadProfilePicture = async (file, accessToken, userId = null) => {
  try {
    const formData = new FormData();
    formData.append('file', file);
    
    let url = `${API_URL}/profile/upload`;
    if (userId) {
      url += `?user_id=${userId}`;
    }
    
    const resp = await axios.post(url, formData, {
      headers: createAuthHeaders(accessToken, 'multipart/form-data')
    });
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Upload failed');
  }
};
