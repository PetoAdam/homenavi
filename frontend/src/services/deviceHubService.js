import http from './httpClient';

const DEVICE_HUB_BASE = '/api/hdp';

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

export async function deleteDevice(deviceId, token, options = {}) {
  if (!deviceId) {
    return { success: false, error: 'Missing device id' };
  }
  const params = new URLSearchParams();
  if (options.force) {
    params.set('force', '1');
  }
  const query = params.toString();
  const suffix = query ? `?${query}` : '';
  return await http.del(`${DEVICE_HUB_BASE}/devices/${deviceId}${suffix}`, { token });
}

export async function listIntegrations(token) {
  return await http.get(`${DEVICE_HUB_BASE}/integrations`, { token });
}

export async function listDevices(token) {
  return await http.get(`${DEVICE_HUB_BASE}/devices`, { token });
}

export async function startPairing(payload, token) {
  if (!payload || typeof payload !== 'object') {
    return { success: false, error: 'Missing pairing payload' };
  }
  return await http.post(`${DEVICE_HUB_BASE}/pairings`, payload, { token });
}

export async function stopPairing(protocol, token) {
  const normalized = typeof protocol === 'string' ? protocol.trim().toLowerCase() : '';
  if (!normalized) {
    return { success: false, error: 'Protocol required' };
  }
  const params = new URLSearchParams({ protocol: normalized }).toString();
  return await http.del(`${DEVICE_HUB_BASE}/pairings?${params}`, { token });
}

export async function listPairings(token) {
  return await http.get(`${DEVICE_HUB_BASE}/pairings`, { token });
}

export async function listPairingConfig(token) {
  return await http.get(`${DEVICE_HUB_BASE}/pairing-config`, { token });
}

export default {
  renameDevice,
  updateDevice,
  setDeviceIcon,
  sendDeviceCommand,
  refreshDevice,
  createDevice,
  deleteDevice,
  listDevices,
  listIntegrations,
  startPairing,
  stopPairing,
  listPairings,
  listPairingConfig,
};
