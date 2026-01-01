import http from './httpClient';

const ERS_BASE = '/api/ers';

export async function listErsHome(token) {
  return http.get(`${ERS_BASE}/home`, { token });
}

export async function listErsRooms(token) {
  return http.get(`${ERS_BASE}/rooms`, { token });
}

export async function createErsRoom(payload, token) {
  if (!payload || typeof payload !== 'object') return { success: false, error: 'Missing room payload' };
  return http.post(`${ERS_BASE}/rooms`, payload, { token });
}

export async function deleteErsRoom(roomId, token) {
  if (!roomId) return { success: false, error: 'Missing room id' };
  return http.del(`${ERS_BASE}/rooms/${roomId}`, { token });
}

export async function patchErsRoom(roomId, patch, token) {
  if (!roomId) return { success: false, error: 'Missing room id' };
  if (!patch || typeof patch !== 'object') return { success: false, error: 'Missing patch payload' };
  return http.patch(`${ERS_BASE}/rooms/${roomId}`, patch, { token });
}

export async function listErsTags(token) {
  return http.get(`${ERS_BASE}/tags`, { token });
}

export async function createErsTag(payload, token) {
  if (!payload || typeof payload !== 'object') return { success: false, error: 'Missing tag payload' };
  return http.post(`${ERS_BASE}/tags`, payload, { token });
}

export async function deleteErsTag(tagId, token) {
  if (!tagId) return { success: false, error: 'Missing tag id' };
  return http.del(`${ERS_BASE}/tags/${tagId}`, { token });
}

export async function listErsDevices(token) {
  return http.get(`${ERS_BASE}/devices`, { token });
}

export async function createErsDevice(payload, token) {
  if (!payload || typeof payload !== 'object') return { success: false, error: 'Missing device payload' };
  return http.post(`${ERS_BASE}/devices`, payload, { token });
}

export async function getErsDevice(deviceId, token) {
  if (!deviceId) return { success: false, error: 'Missing device id' };
  return http.get(`${ERS_BASE}/devices/${deviceId}`, { token });
}

export async function patchErsDevice(deviceId, patch, token) {
  if (!deviceId) return { success: false, error: 'Missing device id' };
  if (!patch || typeof patch !== 'object') return { success: false, error: 'Missing patch payload' };
  return http.patch(`${ERS_BASE}/devices/${deviceId}`, patch, { token });
}

export async function setErsDeviceHdpBindings(deviceId, hdpExternalIds, token) {
  if (!deviceId) return { success: false, error: 'Missing device id' };
  const ids = Array.isArray(hdpExternalIds)
    ? hdpExternalIds.map(v => (typeof v === 'string' ? v.trim() : '')).filter(Boolean)
    : [];
  return http.put(`${ERS_BASE}/devices/${deviceId}/bindings/hdp`, { hdp_external_ids: ids }, { token });
}

export async function setErsDeviceTags(deviceId, tagIds, token) {
  if (!deviceId) return { success: false, error: 'Missing device id' };
  const ids = Array.isArray(tagIds)
    ? tagIds.map(v => (typeof v === 'string' ? v.trim() : '')).filter(Boolean)
    : [];
  return http.put(`${ERS_BASE}/devices/${deviceId}/tags`, { tag_ids: ids }, { token });
}

export async function deleteErsDevice(deviceId, token) {
  if (!deviceId) return { success: false, error: 'Missing device id' };
  return http.del(`${ERS_BASE}/devices/${deviceId}`, { token });
}

export async function resolveErsSelector(selector, token) {
  const sel = typeof selector === 'string' ? selector.trim() : '';
  if (!sel) return { success: false, error: 'Missing selector' };
  return http.post(`${ERS_BASE}/selectors/resolve`, { selector: sel }, { token });
}
