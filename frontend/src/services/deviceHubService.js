import http from './httpClient';

const DEVICE_HUB_BASE = '/api/devicehub';

export async function renameDevice(deviceId, name, token) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  return await http.patch(`${DEVICE_HUB_BASE}/devices/${deviceId}`, { name }, { token });
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

export default {
  renameDevice,
  sendDeviceCommand,
};
