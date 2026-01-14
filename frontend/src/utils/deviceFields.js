export function collectDeviceStateFieldKeys(device) {
  const state = device?.state && typeof device.state === 'object' && !Array.isArray(device.state)
    ? device.state
    : null;
  if (!state) return [];

  const reserved = new Set([
    'schema', 'device_id', 'deviceid', 'external_id', 'externalid', 'protocol', 'topic', 'retained',
    'ts', 'timestamp', 'time', 'received_at', 'receivedat',
    'capabilities',
  ]);

  return Object.keys(state)
    .filter(k => k && !reserved.has(k.toLowerCase()))
    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}
