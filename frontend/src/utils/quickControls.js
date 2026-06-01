import { canToggleDevice } from './groupControls';
import { resolveCommandDeviceId } from './deviceIdentity';

function arrayOrEmpty(value) {
  return Array.isArray(value) ? value : [];
}

function safeString(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function addLookupEntry(map, key, device) {
  const normalized = safeString(key);
  if (!normalized || map.has(normalized)) return;
  map.set(normalized, device);
}

function indexDevice(map, device) {
  if (!device || typeof device !== 'object') return;
  addLookupEntry(map, device.id, device);
  addLookupEntry(map, device.ersId, device);
  addLookupEntry(map, device.hdpId, device);
  arrayOrEmpty(device.hdpIds).forEach((value) => addLookupEntry(map, value, device));
}

function collectGroupMemberRefs(group) {
  const refs = [];
  arrayOrEmpty(group?.deviceIds).forEach((value) => refs.push(value));
  arrayOrEmpty(group?.hdpIds).forEach((value) => refs.push(value));
  arrayOrEmpty(group?.devices).forEach((device) => {
    refs.push(device?.id, device?.ersId, device?.hdpId);
    arrayOrEmpty(device?.hdpIds).forEach((value) => refs.push(value));
  });
  return refs.map(safeString).filter(Boolean);
}

function mergeInventoryDevice(ersDevice, realtimeDevice) {
  if (!ersDevice) return realtimeDevice || null;
  if (!realtimeDevice) return ersDevice;
  return {
    ...realtimeDevice,
    ...ersDevice,
    state: realtimeDevice.state || ersDevice.state,
    inputs: realtimeDevice.inputs || ersDevice.inputs,
    capabilities: realtimeDevice.capabilities || ersDevice.capabilities,
  };
}

export function resolveQuickControlDevices({
  selectedIds,
  selectedGroupIds,
  ersDevices,
  ersGroups,
  realtimeDevices,
}) {
  const ersLookup = new Map();
  arrayOrEmpty(ersDevices).forEach((device) => indexDevice(ersLookup, device));

  const realtimeLookup = new Map();
  arrayOrEmpty(realtimeDevices).forEach((device) => indexDevice(realtimeLookup, device));

  const groupLookup = new Map();
  arrayOrEmpty(ersGroups).forEach((group) => {
    const id = safeString(group?.id);
    if (id) groupLookup.set(id, group);
  });

  const requestedRefs = arrayOrEmpty(selectedIds).map(safeString).filter(Boolean);
  arrayOrEmpty(selectedGroupIds).forEach((groupId) => {
    const group = groupLookup.get(safeString(groupId));
    if (!group) return;
    requestedRefs.push(...collectGroupMemberRefs(group));
  });

  const seenRefs = new Set();
  const resolvedDevices = [];
  requestedRefs.forEach((ref) => {
    if (!ref || seenRefs.has(ref)) return;
    seenRefs.add(ref);

    const ersDevice = ersLookup.get(ref) || null;
    const realtimeDevice = realtimeLookup.get(ref)
      || (safeString(ersDevice?.hdpId) ? realtimeLookup.get(safeString(ersDevice.hdpId)) : null)
      || null;

    const merged = mergeInventoryDevice(ersDevice, realtimeDevice);
    if (!merged) return;
    const commandId = resolveCommandDeviceId(merged);
    if (!commandId || !canToggleDevice(merged)) return;
    if (resolvedDevices.some((device) => resolveCommandDeviceId(device) === commandId)) return;
    resolvedDevices.push(merged);
  });

  return resolvedDevices;
}