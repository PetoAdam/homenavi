function normalizeId(value) {
  return typeof value === 'string' ? value.trim() : '';
}

export function resolveCommandDeviceId(device) {
  const directHdpId = normalizeId(device?.hdpId);
  if (directHdpId) return directHdpId;

  const hdpIds = Array.isArray(device?.hdpIds) ? device.hdpIds : [];
  const listedHdpId = hdpIds.map(normalizeId).find(Boolean);
  if (listedHdpId) return listedHdpId;

  const fallbackIds = [
    device?.device_id,
    device?.id,
    device?.externalId,
  ];

  return fallbackIds.map(normalizeId).find(Boolean) || '';
}