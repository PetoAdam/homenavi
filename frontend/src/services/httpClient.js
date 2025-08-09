import axios from 'axios';

// Unified HTTP client with consistent error shaping & auth header injection.
// Usage: http.get(url, { token, params }); http.post(url, data, { token });

const instance = axios.create({
  // baseURL could be set if gateway path prefix is common; keep relative for proxy flexibility.
  timeout: 15000,
});

// Access token in-memory cache (AuthContext should also update this)
let currentAccessToken = null;
export function setAccessToken(token) { currentAccessToken = token; }

// Flag to avoid infinite retry loops
let isRefreshing = false;
let refreshQueue = [];

async function processQueue(error, token = null) {
  refreshQueue.forEach(p => {
    if (error) p.reject(error); else p.resolve(token);
  });
  refreshQueue = [];
}

instance.interceptors.request.use(cfg => {
  if (currentAccessToken && !cfg.headers['Authorization']) {
    cfg.headers['Authorization'] = `Bearer ${currentAccessToken}`;
  }
  return cfg;
});

instance.interceptors.response.use(r => r, async (error) => {
  const { response, config } = error || {};
  if (!response) return Promise.reject(error);
  // Only attempt refresh on 401 and once per request
  if (response.status === 401 && !config._retry) {
    const refreshToken = localStorage.getItem('refreshToken');
    if (!refreshToken) return Promise.reject(error);
    if (isRefreshing) {
      return new Promise((resolve, reject) => {
        refreshQueue.push({ resolve, reject });
      }).then((newToken) => {
        config.headers['Authorization'] = `Bearer ${newToken}`;
        return instance(config);
      });
    }
    config._retry = true;
    isRefreshing = true;
    try {
      const resp = await axios.post('/api/auth/refresh', { refresh_token: refreshToken });
      const newAccess = resp.data.access_token;
      const newRefresh = resp.data.refresh_token;
      if (newAccess) {
        currentAccessToken = newAccess;
        localStorage.setItem('refreshToken', newRefresh || refreshToken);
        // Update auth cookie for websockets
        document.cookie = `auth_token=${newAccess}; path=/; SameSite=strict; max-age=900`;
        processQueue(null, newAccess);
        config.headers['Authorization'] = `Bearer ${newAccess}`;
        return instance(config);
      }
      processQueue(new Error('No access token in refresh response'));
      return Promise.reject(error);
    } catch (e) {
      processQueue(e);
      // Clear tokens on refresh failure
      currentAccessToken = null;
      localStorage.removeItem('refreshToken');
      document.cookie = 'auth_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT';
      return Promise.reject(e);
    } finally {
      isRefreshing = false;
    }
  }
  return Promise.reject(error);
});

// Standard error normalizer
function normalizeError(err, fallback) {
  const resp = err?.response;
  const data = resp?.data;
  return {
    success: false,
    status: resp?.status,
    error: data?.error || data?.message || err.message || fallback,
    raw: err,
  };
}

function authHeaders(token, contentType = 'application/json') {
  const h = {};
  if (token) h['Authorization'] = `Bearer ${token}`;
  if (contentType) h['Content-Type'] = contentType;
  return h;
}

// Core request helpers returning { success, ... }
async function get(url, { token, params, responseType } = {}) {
  try {
    const resp = await instance.get(url, { params, responseType, headers: authHeaders(token) });
    return { success: true, data: resp.data, status: resp.status };
  } catch (err) { return normalizeError(err, 'GET failed'); }
}

async function del(url, { token, data } = {}) {
  try {
    const resp = await instance.delete(url, { data, headers: authHeaders(token) });
    return { success: true, data: resp.data, status: resp.status };
  } catch (err) { return normalizeError(err, 'DELETE failed'); }
}

async function post(url, data, { token, contentType = 'application/json', params } = {}) {
  try {
    const resp = await instance.post(url, data, { params, headers: authHeaders(token, contentType) });
    return { success: true, data: resp.data, status: resp.status };
  } catch (err) { return normalizeError(err, 'POST failed'); }
}

async function patch(url, data, { token, contentType = 'application/json' } = {}) {
  try {
    const resp = await instance.patch(url, data, { headers: authHeaders(token, contentType) });
    return { success: true, data: resp.data, status: resp.status };
  } catch (err) { return normalizeError(err, 'PATCH failed'); }
}

export const http = { get, post, patch, del, authHeaders, setAccessToken };
export default http;
