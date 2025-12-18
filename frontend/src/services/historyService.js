import http from './httpClient';

const HISTORY_BASE = '/api/history';

export async function health(token) {
  return await http.get(`${HISTORY_BASE}/health`, { token });
}

export async function listStatePoints(deviceId, options = {}, token) {
  const normalizedId = typeof deviceId === 'string' ? deviceId.trim() : '';
  if (!normalizedId) {
    return { success: false, error: 'Missing device id' };
  }

  const params = {
    device_id: normalizedId,
  };

  if (options.from) params.from = options.from;
  if (options.to) params.to = options.to;
  if (options.limit) params.limit = options.limit;
  if (options.cursor) params.cursor = options.cursor;
  if (options.order) params.order = options.order;

  return await http.get(`${HISTORY_BASE}/state`, { token, params });
}

export default {
  health,
  listStatePoints,
};
