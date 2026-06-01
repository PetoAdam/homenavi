import { sanitizeInputKey, toControlBoolean } from '../components/common/DeviceControlRenderer/deviceControlUtils';
import { canToggleDevice, intersectSharedInputs } from './groupControls';
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

function findPrimaryToggleInput(inputs) {
  const list = arrayOrEmpty(inputs);
  return list.find((input) => {
    if (input?.type !== 'toggle') return false;
    const key = sanitizeInputKey(input).toLowerCase();
    return key === 'on' || key === 'state' || key === 'power';
  }) || list.find((input) => input?.type === 'toggle') || null;
}

function buildDeviceLookups(ersDevices, realtimeDevices) {
  const ersLookup = new Map();
  arrayOrEmpty(ersDevices).forEach((device) => indexDevice(ersLookup, device));

  const realtimeLookup = new Map();
  arrayOrEmpty(realtimeDevices).forEach((device) => indexDevice(realtimeLookup, device));

  return { ersLookup, realtimeLookup };
}

function resolveInventoryDevice(ref, ersLookup, realtimeLookup) {
  const ersDevice = ersLookup.get(ref) || null;
  const realtimeDevice = realtimeLookup.get(ref)
    || (safeString(ersDevice?.hdpId) ? realtimeLookup.get(safeString(ersDevice.hdpId)) : null)
    || null;
  return mergeInventoryDevice(ersDevice, realtimeDevice);
}

function resolveGroupDevices(group, ersLookup, realtimeLookup) {
  const devices = [];
  const seenCommandIds = new Set();

  collectGroupMemberRefs(group).forEach((ref) => {
    const merged = resolveInventoryDevice(ref, ersLookup, realtimeLookup);
    if (!merged) return;
    const commandId = resolveCommandDeviceId(merged);
    if (!commandId || seenCommandIds.has(commandId)) return;
    seenCommandIds.add(commandId);
    devices.push(merged);
  });

  if (devices.length > 0) return devices;

  arrayOrEmpty(group?.devices).forEach((device) => {
    const commandId = resolveCommandDeviceId(device);
    if (!commandId || seenCommandIds.has(commandId)) return;
    seenCommandIds.add(commandId);
    devices.push(device);
  });

  return devices;
}

export function resolveQuickControlGroup(group, ersLookup = new Map(), realtimeLookup = new Map()) {
  if (!group || typeof group !== 'object') return null;

  const devices = resolveGroupDevices(group, ersLookup, realtimeLookup);
  if (!devices.length) return null;

  const sharedControls = intersectSharedInputs(devices);
  const toggleInput = findPrimaryToggleInput(sharedControls.inputs);
  if (!toggleInput) return null;

  const toggleKey = sanitizeInputKey(toggleInput);
  if (!toggleKey) return null;

  return {
    kind: 'group',
    key: `group:${safeString(group.id) || safeString(group.slug) || safeString(group.name)}`,
    id: safeString(group.id),
    group: {
      ...group,
      devices,
    },
    toggleInput,
    toggleKey,
    toggleValue: toControlBoolean(sharedControls.values[toggleKey]),
    mixed: sharedControls.mixedKeys.includes(toggleKey),
    annotation: sharedControls.mixedAnnotations[toggleKey] || '',
  };
}

export function canToggleGroup(group, ersLookup = new Map(), realtimeLookup = new Map()) {
  return Boolean(resolveQuickControlGroup(group, ersLookup, realtimeLookup));
}

export function resolveQuickControlItems({
  selectedIds,
  selectedGroupIds,
  ersDevices,
  ersGroups,
  realtimeDevices,
}) {
  const { ersLookup, realtimeLookup } = buildDeviceLookups(ersDevices, realtimeDevices);

  const groupLookup = new Map();
  arrayOrEmpty(ersGroups).forEach((group) => {
    const id = safeString(group?.id);
    if (id) groupLookup.set(id, group);
  });

  const resolvedItems = [];

  arrayOrEmpty(selectedIds).map(safeString).filter(Boolean).forEach((ref) => {
    const merged = resolveInventoryDevice(ref, ersLookup, realtimeLookup);
    if (!merged) return;
    const commandId = resolveCommandDeviceId(merged);
    if (!commandId || !canToggleDevice(merged)) return;
    if (resolvedItems.some((item) => item.kind === 'device' && item.commandId === commandId)) return;
    resolvedItems.push({
      kind: 'device',
      key: `device:${commandId}`,
      commandId,
      device: merged,
    });
  });

  arrayOrEmpty(selectedGroupIds).map(safeString).filter(Boolean).forEach((groupId) => {
    const group = groupLookup.get(groupId);
    if (!group) return;
    const resolvedGroup = resolveQuickControlGroup(group, ersLookup, realtimeLookup);
    if (!resolvedGroup) return;
    if (resolvedItems.some((item) => item.kind === 'group' && item.id === resolvedGroup.id)) return;
    resolvedItems.push(resolvedGroup);
  });

  return resolvedItems;
}