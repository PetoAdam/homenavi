import { sanitizeInputKey, toControlBoolean } from '../components/common/DeviceControlRenderer/deviceControlUtils';
import { normalizeColorHex } from './colorHex';
import { collectDeviceStateFieldKeys } from './deviceFields';

function arrayOrEmpty(value) {
  return Array.isArray(value) ? value : [];
}

function getCapabilityKeyParts(cap) {
  if (!cap || typeof cap !== 'object') return [];
  return [cap.id, cap.property, cap.name]
    .filter((part) => part !== undefined && part !== null)
    .map((part) => part.toString().toLowerCase());
}

function collectCapabilities(device) {
  const lists = [
    arrayOrEmpty(device?.capabilities),
    arrayOrEmpty(device?.state?.capabilities),
  ];
  const seen = new Set();
  const result = [];
  lists.flat().forEach((cap) => {
    if (!cap || typeof cap !== 'object') return;
    const keys = getCapabilityKeyParts(cap);
    const primary = keys[0];
    if (primary && seen.has(primary)) return;
    if (keys.length === 0) {
      result.push(cap);
      return;
    }
    keys.forEach((key) => seen.add(key));
    result.push(cap);
  });
  return result;
}

function isCapabilityWritable(cap) {
  if (!cap || typeof cap !== 'object') return false;
  const access = cap.access || {};
  if (access.readOnly === true) return false;
  const negativeFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === false);
  if (negativeFlags.length > 0) return false;
  const positiveFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === true);
  if (positiveFlags.length > 0) return true;
  if ('write' in access || 'set' in access || 'command' in access || 'toggle' in access) {
    return Boolean(access.write || access.set || access.command || access.toggle);
  }
  return false;
}

function inferInputTypeFromCapability(cap) {
  const kind = (cap.kind || '').toString().toLowerCase();
  const valueType = (cap.value_type || cap.valueType || '').toString().toLowerCase();
  const name = (cap.name || '').toString().toLowerCase();
  const property = (cap.property || '').toString().toLowerCase();
  if (property === 'color' || name.includes('color')) return 'color';
  if (kind === 'binary' || valueType === 'boolean') return 'toggle';
  if (kind === 'enum' || valueType === 'enum' || Array.isArray(cap.enum)) return 'select';
  if (kind === 'numeric' || valueType === 'number') {
    if (typeof cap.range?.min === 'number' && typeof cap.range?.max === 'number') return 'slider';
    return 'number';
  }
  if (valueType === 'object' && (name.includes('color') || property === 'color')) return 'color';
  return null;
}

function formatDisplayLabel(value) {
  if (!value) return '';
  return value
    .toString()
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function buildInputsFromCapabilities(capabilities) {
  const results = [];
  capabilities.forEach((cap) => {
    if (!cap || typeof cap !== 'object') return;
    if (!isCapabilityWritable(cap)) return;
    const inferredType = inferInputTypeFromCapability(cap);
    if (!inferredType) return;
    const rawId = cap.id || cap.property || cap.name;
    if (!rawId) return;
    const capabilityId = cap.id || cap.property || rawId;
    const property = cap.property || capabilityId;
    const range = cap.range && typeof cap.range === 'object'
      ? {
        min: typeof cap.range.min === 'number' ? cap.range.min : undefined,
        max: typeof cap.range.max === 'number' ? cap.range.max : undefined,
        step: typeof cap.range.step === 'number' ? cap.range.step : (typeof cap.step === 'number' ? cap.step : undefined),
      }
      : null;
    const options = Array.isArray(cap.enum)
      ? cap.enum.map((value) => ({ value, label: formatDisplayLabel(value) }))
      : [];
    results.push({
      id: rawId,
      type: inferredType,
      property,
      capability_id: capabilityId,
      label: cap.name || cap.property || rawId,
      range,
      options,
      metadata: { ...(cap.metadata || {}), fromCapability: true },
      access: cap.access || {},
      capability: cap,
      readOnly: false,
    });
  });
  return results;
}

function buildCapabilityLookup(capabilities) {
  const map = new Map();
  capabilities.forEach((cap) => {
    getCapabilityKeyParts(cap).forEach((key) => {
      if (key && !map.has(key)) map.set(key, cap);
    });
  });
  return map;
}

function resolveCapabilityForInput(capabilityLookup, input) {
  if (!input || !capabilityLookup) return null;
  const candidates = [input.capability_id, input.capabilityId, input.capabilityID, input.id, input.property];
  for (const candidate of candidates) {
    if (!candidate) continue;
    const key = candidate.toString().toLowerCase();
    if (capabilityLookup.has(key)) return capabilityLookup.get(key);
  }
  return null;
}

function resolveStateValue(state, cap) {
  if (!state) return undefined;
  const candidates = [cap.id, cap.property, cap.name].filter(Boolean).map((key) => key.toString());
  for (const key of candidates) {
    if (state[key] !== undefined) return state[key];
    const lower = key.toLowerCase();
    if (state[lower] !== undefined) return state[lower];
    const camel = lower.replace(/_([a-z])/g, (_, char) => char.toUpperCase());
    if (state[camel] !== undefined) return state[camel];
  }
  return undefined;
}

function extractStateColorHex(raw) {
  const normalized = normalizeColorHex(raw, '');
  return normalized || null;
}

export function getStateValueForInput(device, input) {
  if (!device || !input) return undefined;
  const state = device.state || {};
  const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || input.id;
  const pseudoCap = {
    id: capabilityId || input.id || input.property,
    property: input.property || capabilityId || input.id,
    name: input.label || input.property,
  };
  return resolveStateValue(state, pseudoCap);
}

export function buildInitialControlValues(device, inputs) {
  const values = {};
  arrayOrEmpty(inputs).forEach((input) => {
    const key = sanitizeInputKey(input);
    if (!key) return;
    let raw = getStateValueForInput(device, input);
    if (raw === undefined && input.type === 'toggle' && typeof device?.toggleState === 'boolean') raw = device.toggleState;
    if (raw === undefined && input.type === 'slider' && input.range && typeof input.range.min === 'number') raw = input.range.min;
    if (raw === undefined && input.type === 'select' && input.options?.length) raw = input.options[0].value;
    switch (input.type) {
    case 'toggle':
      values[key] = toControlBoolean(raw);
      break;
    case 'slider':
    case 'number':
      values[key] = typeof raw === 'number' ? raw : Number(raw ?? 0);
      break;
    case 'color':
      values[key] = extractStateColorHex(raw) || '#FFFFFF';
      break;
    case 'select':
      values[key] = raw ?? '';
      break;
    default:
      if (raw !== undefined) values[key] = raw;
    }
  });
  return values;
}

function normalizeToggleProperty(property) {
  const key = (property ?? '').toString().trim();
  const lower = key.toLowerCase();
  if (lower === 'state' || lower === 'power' || lower === 'on') return 'on';
  return key;
}

export function buildPayloadForInput(input, value) {
  const key = sanitizeInputKey(input);
  if (!key) return null;
  const stateKeyRaw = (input.property || key || '').toString().trim();
  if (!stateKeyRaw) return null;
  const state = {};
  const propertyLower = stateKeyRaw.toLowerCase();
  switch (input.type) {
  case 'toggle':
    if (input?.metadata?.togglePowerString === true || propertyLower === 'power') {
      state.power = toControlBoolean(value) ? 'on' : 'off';
    } else {
      state[normalizeToggleProperty(stateKeyRaw)] = toControlBoolean(value);
    }
    break;
  case 'slider':
  case 'number': {
    const numeric = Number(value);
    if (Number.isNaN(numeric)) return null;
    state[stateKeyRaw] = numeric;
    if (propertyLower.startsWith('brightness')) state.on = numeric > 0;
    break;
  }
  case 'color':
    state[stateKeyRaw] = value;
    state.on = true;
    break;
  case 'select':
    state[stateKeyRaw] = value;
    if (propertyLower.includes('effect')) state.on = true;
    break;
  default:
    state[stateKeyRaw] = value;
  }
  return { state };
}

function buildNormalizedInputs(device) {
  const capabilities = collectCapabilities(device);
  const capabilityLookup = buildCapabilityLookup(capabilities);
  const rawList = arrayOrEmpty(device?.inputs);
  const hasCompositeColor = rawList.some((input) => (input?.type || '').toLowerCase() === 'color' && input?.metadata?.mode === 'composite');
  const suppressedProperties = new Set();
  if (hasCompositeColor) {
    ['x', 'y', 'hue', 'saturation', 'color_x', 'color_y', 'color_hs', 'color_xy'].forEach((key) => suppressedProperties.add(key));
  }
  const normalized = rawList
    .map((input) => {
      const originalType = (input.type || '').toLowerCase();
      const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || '';
      const property = input.property || capabilityId || input.id || '';
      const propertyLower = property.toString().toLowerCase();
      const options = Array.isArray(input.options) ? input.options : [];
      const hasOn = options.some((opt) => String(opt?.value || '').toLowerCase() === 'on');
      const hasOff = options.some((opt) => String(opt?.value || '').toLowerCase() === 'off');
      const isPowerSelect = originalType === 'select' && propertyLower === 'power' && hasOn && hasOff;
      const type = isPowerSelect ? 'toggle' : originalType;
      const id = input.id || capabilityId || property;
      const capability = resolveCapabilityForInput(capabilityLookup, input);
      const access = {
        ...(capability?.access || {}),
        ...(input.metadata?.access || {}),
        ...(input.access || {}),
      };
      const readOnly = Boolean(access?.readOnly)
        || access.write === false
        || access.set === false
        || access.toggle === false
        || (capability ? !isCapabilityWritable(capability) : false);
      return {
        ...input,
        id,
        type,
        property,
        capability_id: capabilityId,
        capability,
        access,
        readOnly,
        options,
        metadata: {
          ...(input.metadata || {}),
          ...(isPowerSelect ? { togglePowerString: true } : {}),
        },
        range: input.range || input.Range || null,
      };
    })
    .filter((input) => {
      if (!input.id || input.readOnly) return false;
      if (!['toggle', 'slider', 'number', 'select', 'color'].includes(input.type)) return false;
      const propertyKey = (input.property || input.id || '').toString().toLowerCase();
      if (suppressedProperties.has(propertyKey)) return false;
      return true;
    });
  const seenKeys = new Set(normalized.map((item) => sanitizeInputKey(item).toLowerCase()));
  buildInputsFromCapabilities(capabilities).forEach((fallback) => {
    const key = sanitizeInputKey(fallback).toLowerCase();
    if (!key || seenKeys.has(key)) return;
    const propertyKey = (fallback.property || fallback.id || '').toString().toLowerCase();
    if (suppressedProperties.has(propertyKey)) return;
    seenKeys.add(key);
    normalized.push({
      ...fallback,
      options: Array.isArray(fallback.options) ? fallback.options : [],
      metadata: fallback.metadata || {},
      range: fallback.range || null,
      capability: fallback.capability,
      capability_id: fallback.capability_id,
      access: fallback.access || {},
      readOnly: false,
    });
  });
  return normalized;
}

function valuesEqualByType(type, left, right) {
  if (type === 'color') return extractStateColorHex(left) === extractStateColorHex(right);
  if (type === 'toggle') return toControlBoolean(left) === toControlBoolean(right);
  return String(left ?? '') === String(right ?? '');
}

function formatMixedValuePreview(type, value) {
  if (value === undefined || value === null || value === '') return null;
  if (type === 'toggle') return toControlBoolean(value) ? 'on' : 'off';
  if (type === 'color') return extractStateColorHex(value)?.toUpperCase() || null;
  if (typeof value === 'number') return Number.isInteger(value) ? String(value) : String(Math.round(value * 10) / 10);
  return String(value).trim() || null;
}

function buildMixedControlAnnotation(input, values) {
  const label = (typeof input?.label === 'string' ? input.label.trim() : '') || formatDisplayLabel(input?.property || input?.id || 'value');
  const prefix = `Mixed ${label.toLowerCase()}`;
  const previews = Array.from(new Set(values.map((value) => formatMixedValuePreview(input?.type, value)).filter(Boolean)));
  if (previews.length >= 2 && previews.length <= 3) {
    return `${prefix}: ${previews.join(' / ')}`;
  }
  return prefix;
}

export function intersectSharedInputs(devices) {
  const members = arrayOrEmpty(devices);
  if (!members.length) return { inputs: [], values: {}, mixedKeys: [], mixedAnnotations: {} };
  const normalizedByDevice = members.map((device) => ({ device, inputs: buildNormalizedInputs(device) }));
  if (normalizedByDevice.some((entry) => entry.inputs.length === 0)) {
    return { inputs: [], values: {}, mixedKeys: [], mixedAnnotations: {} };
  }
  const maps = normalizedByDevice.map((entry) => {
    const map = new Map();
    entry.inputs.forEach((input) => {
      const key = sanitizeInputKey(input).toLowerCase();
      if (key) map.set(key, input);
    });
    return map;
  });
  const sharedKeys = Array.from(maps[0].keys()).filter((key) => maps.every((map) => map.has(key)));
  const sharedInputs = [];
  const sharedValues = {};
  const mixedKeys = [];
  const mixedAnnotations = {};

  sharedKeys.forEach((key) => {
    const candidates = maps.map((map) => map.get(key)).filter(Boolean);
    if (!candidates.length) return;
    const base = { ...candidates[0] };
    if (!candidates.every((candidate) => candidate.type === base.type)) return;
    if (base.type === 'select') {
      const optionSets = candidates.map((candidate) => new Map(arrayOrEmpty(candidate.options).map((option) => [String(option.value).toLowerCase(), option])));
      const intersectedOptions = arrayOrEmpty(base.options).filter((option) => optionSets.every((set) => set.has(String(option.value).toLowerCase())));
      if (!intersectedOptions.length) return;
      base.options = intersectedOptions;
    }
    if (base.type === 'slider' || base.type === 'number') {
      const mins = candidates.map((candidate) => candidate.range?.min).filter((value) => typeof value === 'number');
      const maxes = candidates.map((candidate) => candidate.range?.max).filter((value) => typeof value === 'number');
      const steps = candidates.map((candidate) => candidate.range?.step).filter((value) => typeof value === 'number');
      const min = mins.length ? Math.max(...mins) : undefined;
      const max = maxes.length ? Math.min(...maxes) : undefined;
      if (typeof min === 'number' && typeof max === 'number' && max < min) return;
      base.range = {
        ...(base.range || {}),
        ...(typeof min === 'number' ? { min } : {}),
        ...(typeof max === 'number' ? { max } : {}),
        ...(steps.length ? { step: Math.max(...steps) } : {}),
      };
    }
    sharedInputs.push(base);
    const memberValues = normalizedByDevice.map(({ device }) => getStateValueForInput(device, base));
    const comparableValues = memberValues.filter((value) => value !== undefined);
    const initialValues = buildInitialControlValues(normalizedByDevice[0].device, [base]);
    const baseValue = comparableValues.length ? comparableValues[0] : initialValues[sanitizeInputKey(base)];
    sharedValues[sanitizeInputKey(base)] = base.type === 'color'
      ? extractStateColorHex(baseValue) || initialValues[sanitizeInputKey(base)]
      : base.type === 'toggle'
        ? toControlBoolean(baseValue)
        : baseValue;
    if (comparableValues.length > 1 && !comparableValues.every((value) => valuesEqualByType(base.type, comparableValues[0], value))) {
      const inputKey = sanitizeInputKey(base);
      mixedKeys.push(inputKey);
      mixedAnnotations[inputKey] = buildMixedControlAnnotation(base, comparableValues);
      base.mixed = true;
      base.annotation = mixedAnnotations[inputKey];
    }
  });

  return { inputs: sharedInputs, values: sharedValues, mixedKeys, mixedAnnotations };
}

function valuesEqual(left, right) {
  if (left === right) return true;
  if (typeof left === 'object' || typeof right === 'object') {
    return JSON.stringify(left ?? null) === JSON.stringify(right ?? null);
  }
  return String(left ?? '') === String(right ?? '');
}

export function collectCommonFieldKeys(devices) {
  const members = arrayOrEmpty(devices);
  if (!members.length) return [];
  const fieldLists = members.map((device) => collectDeviceStateFieldKeys(device));
  if (fieldLists.some((fields) => fields.length === 0)) return [];
  return fieldLists[0].filter((key) => fieldLists.every((fields) => fields.includes(key)));
}

export function buildSharedFieldState(devices, selectedFields = null) {
  const commonKeys = collectCommonFieldKeys(devices);
  const allowed = Array.isArray(selectedFields)
    ? selectedFields.filter((key) => commonKeys.includes(key))
    : commonKeys;
  const state = {};
  allowed.forEach((key) => {
    const values = arrayOrEmpty(devices)
      .map((device) => (device?.state && typeof device.state === 'object' ? device.state[key] : undefined))
      .filter((value) => value !== undefined);
    if (!values.length) return;
    state[key] = values.every((value) => valuesEqual(values[0], value)) ? values[0] : 'Mixed';
  });
  return state;
}