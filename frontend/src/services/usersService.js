import axios from 'axios';

const API_USERS_URL = '/api/users';

const headers = (token, contentType = 'application/json') => ({
  'Authorization': `Bearer ${token}`,
  'Content-Type': contentType,
});

const handleError = (err, defaultMessage) => {
  console.error(defaultMessage, err);
  const errorMessage = err.response?.data?.error || err.response?.data?.message || err.message || defaultMessage;
  return { success: false, error: errorMessage };
};

export async function listUsers({ q = '', page = 1, pageSize = 20 } = {}, token) {
  try {
    const resp = await axios.get(API_USERS_URL, {
      headers: headers(token),
      params: {
        q: q || undefined,
        page,
        page_size: pageSize,
      },
    });
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Failed to fetch users');
  }
}

export async function updateUser(userId, patch, token) {
  try {
    await axios.patch(`${API_USERS_URL}/${userId}`, patch, { headers: headers(token) });
    return { success: true };
  } catch (err) {
    return handleError(err, 'Failed to update user');
  }
}

export async function lockoutUser(userId, lock, token) {
  try {
    await axios.post(`${API_USERS_URL}/${userId}/lockout`, { lock }, { headers: headers(token) });
    return { success: true };
  } catch (err) {
    return handleError(err, 'Failed to update lockout');
  }
}

export async function getUserByEmail(email, token) {
  try {
    const resp = await axios.get(API_USERS_URL, { headers: headers(token), params: { email } });
    return { success: true, data: resp.data };
  } catch (err) {
    return handleError(err, 'Failed to fetch user');
  }
}
