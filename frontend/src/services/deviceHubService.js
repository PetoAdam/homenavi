import http from './httpClient';

const DEVICE_HUB_BASE = '/api/devicehub';

export async function updateDevice(deviceId, payload, token) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  if (!payload || typeof payload !== 'object') {
    return { success: false, error: 'Missing update payload' };
  }
  return await http.patch(`${DEVICE_HUB_BASE}/devices/${deviceId}`, payload, { token });
}

export async function renameDevice(deviceId, name, token) {
  return updateDevice(deviceId, { name }, token);
}

export async function setDeviceIcon(deviceId, icon, token) {
  const normalized = typeof icon === 'string' ? icon.trim() : '';
  const payload = normalized ? { icon: normalized } : { icon: '' };
  return updateDevice(deviceId, payload, token);
}

export async function sendDeviceCommand(deviceId, payload, token) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  if (!payload || typeof payload !== 'object') {
    return { success: false, error: 'Missing command payload' };
  }
  return await http.post(`${DEVICE_HUB_BASE}/devices/${deviceId}/commands`, payload, { token });
}

export async function refreshDevice(deviceId, options = {}, token) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  return await http.post(`${DEVICE_HUB_BASE}/devices/${deviceId}/refresh`, options, { token });
}

export async function createDevice(payload, token) {
  return await http.post(`${DEVICE_HUB_BASE}/devices`, payload, { token });
}

export async function deleteDevice(deviceId, token) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  return await http.del(`${DEVICE_HUB_BASE}/devices/${deviceId}`, { token });
}

export async function listIntegrations(token) {
  return await http.get(`${DEVICE_HUB_BASE}/integrations`, { token });
}

export default {
  renameDevice,
  updateDevice,
  setDeviceIcon,
  sendDeviceCommand,
  refreshDevice,
  createDevice,
  deleteDevice,
  listIntegrations,
};
