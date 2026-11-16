import { getDisplayPresentationOptions, isValidControlOverride, normalizeInputType, sanitizeInputKey, toControlBoolean } from '../components/common/DeviceControlRenderer/deviceControlUtils';
import { normalizeColorHex } from './colorHex';
import { collectDeviceStateFieldKeys } from './deviceFields';
import { applyInputPresentation, inputIdentityKeys } from './devicePresentation';

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
    if (raw === undefined && input.type === 'select' && input.options?.length) raw = input.options[0].value;
    switch (input.type) {
    case 'toggle':
      values[key] = toControlBoolean(raw);
      break;
    case 'slider':
    case 'number':
      values[key] = raw == null || raw === '' ? '' : Number(raw);
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

export function buildNormalizedInputs(device) {
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
      const originalType = normalizeInputType(input.type);
      const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || '';
      const property = input.property || capabilityId || input.id || '';
      const propertyLower = property.toString().toLowerCase();
      const options = Array.isArray(input.options) ? input.options : [];
      const hasOn = options.some((opt) => String(opt?.value || '').toLowerCase() === 'on');
      const hasOff = options.some((opt) => String(opt?.value || '').toLowerCase() === 'off');
      const capability = resolveCapabilityForInput(capabilityLookup, input);
      const isPowerSelect = originalType === 'select' && propertyLower === 'power' && hasOn && hasOff && !isValidControlOverride({ ...input, capability });
      const type = isPowerSelect ? 'toggle' : originalType;
      const id = input.id || capabilityId || property;
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
      if (!['toggle', 'slider', 'number', 'select', 'color', 'text'].includes(input.type)) return false;
      const propertyKey = (input.property || input.id || '').toString().toLowerCase();
      if (suppressedProperties.has(propertyKey)) return false;
      return true;
    });
  const seenKeys = new Set(normalized.flatMap(inputIdentityKeys));
  buildInputsFromCapabilities(capabilities).forEach((fallback) => {
    const keys = inputIdentityKeys(fallback);
    if (!keys.length || keys.some(key => seenKeys.has(key))) return;
    const propertyKey = (fallback.property || fallback.id || '').toString().toLowerCase();
    if (suppressedProperties.has(propertyKey)) return;
    keys.forEach(key => seenKeys.add(key));
    normalized.push(applyInputPresentation({
      ...fallback,
      options: Array.isArray(fallback.options) ? fallback.options : [],
      metadata: fallback.metadata || {},
      range: fallback.range || null,
      capability: fallback.capability,
      capability_id: fallback.capability_id,
      access: fallback.access || {},
      readOnly: false,
    }, device?.meta));
  });
  return normalized;
}

export function buildPresentationInputs(device) {
  return buildNormalizedInputs(device).map(input => {
    const raw = arrayOrEmpty(device?.inputs).find(item => sanitizeInputKey(item) === sanitizeInputKey(input));
    return raw && input.metadata?.togglePowerString ? { ...input, ...raw, capability: input.capability } : input;
  });
}

export function findToggleInput(device) {
  const normalizedInputs = buildNormalizedInputs(device);
  return normalizedInputs.find((input) => {
    if (input?.type !== 'toggle') return false;
    const key = sanitizeInputKey(input).toLowerCase();
    return key === 'on' || key === 'state' || key === 'power';
  }) || normalizedInputs.find((input) => input?.type === 'toggle') || null;
}

export function canToggleDevice(device) {
  return Boolean(findToggleInput(device));
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

export function intersectNumericRanges(candidates) {
  if (!candidates.length) return null;
  const ranges = candidates.map(candidate => candidate.range || {});
  if (ranges.some(range => range.except !== undefined && (!Array.isArray(range.except) || range.except.some(value => value === '' || !Number.isFinite(Number(value)))))) return null;
  const excluded = [...new Set(ranges.flatMap(range => (range.except || []).map(Number)))];
  const numbers = ranges.flatMap(range => [range.min, range.max, range.step].filter(value => value !== undefined));
  if (numbers.some(value => typeof value !== 'number' || !Number.isFinite(value)) || ranges.some(range => range.step !== undefined && range.step <= 0)) return null;
  let scale = 1;
  while (scale <= 1000000000000000 && numbers.some(value => Math.round(value * scale) / scale !== value)) scale *= 10;
  if (scale > 1000000000000000 || numbers.some(value => !Number.isSafeInteger(Math.round(value * scale)))) return null;
  const scaled = ranges.map(range => ({
    min: range.min === undefined ? undefined : BigInt(Math.round(range.min * scale)),
    max: range.max === undefined ? undefined : BigInt(Math.round(range.max * scale)),
    step: BigInt(Math.round((range.step ?? 1) * scale)),
  }));
  if (scaled.some(range => range.step <= 0n)) return null;
  const mins = scaled.map(range => range.min).filter(value => value !== undefined);
  const maxes = scaled.map(range => range.max).filter(value => value !== undefined);
  const lower = mins.length ? mins.reduce((left, right) => left > right ? left : right) : undefined;
  const upper = maxes.length ? maxes.reduce((left, right) => left < right ? left : right) : undefined;
  if (lower !== undefined && upper !== undefined && lower > upper) return null;
  const modulo = (value, modulus) => ((value % modulus) + modulus) % modulus;
  const extendedGcd = (left, right) => {
    let previous = 1n;
    let current = 0n;
    while (right !== 0n) {
      const quotient = left / right;
      [left, right] = [right, left % right];
      [previous, current] = [current, previous - quotient * current];
    }
    return [left, previous];
  };
  let origin = modulo(scaled[0].min ?? 0n, scaled[0].step);
  let step = scaled[0].step;
  for (const range of scaled.slice(1)) {
    const [divisor, inverse] = extendedGcd(step, range.step);
    const difference = (range.min ?? 0n) - origin;
    if (difference % divisor !== 0n) return null;
    const multiplier = modulo((difference / divisor) * inverse, range.step / divisor);
    origin += step * multiplier;
    step = (step / divisor) * range.step;
    origin = modulo(origin, step);
  }
  const first = lower !== undefined ? lower + modulo(origin - lower, step)
    : upper !== undefined ? upper - modulo(upper - origin, step) : origin;
  if ((upper !== undefined && first > upper) || step > BigInt(Number.MAX_SAFE_INTEGER)
    || first > BigInt(Number.MAX_SAFE_INTEGER) || first < BigInt(Number.MIN_SAFE_INTEGER)) return null;
  if (lower !== undefined && upper !== undefined) {
    let available = first;
    while (available <= upper && excluded.includes(Number(available) / scale)) available += step;
    if (available > upper) return null;
  }
  return {
    ...(candidates[0].range || {}),
    ...(lower !== undefined ? { min: Number(first) / scale } : {}),
    ...(upper !== undefined ? { max: Number(upper) / scale } : {}),
    step: Number(step) / scale,
    ...(excluded.length ? { except: excluded } : {}),
  };
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
      if (key) map.set(key, map.has(key) ? null : input);
    });
    return map;
  });
  const sharedKeys = Array.from(maps[0].keys()).filter((key) => maps.every((map) => map.get(key)));
  const sharedInputs = [];
  const sharedValues = {};
  const mixedKeys = [];
  const mixedAnnotations = {};

  sharedKeys.forEach((key) => {
    let candidates = maps.map((map) => map.get(key)).filter(Boolean);
    if (!candidates.length) return;
    if (!candidates.every(candidate => candidate.property === candidates[0].property)) return;
    if (!candidates.every(candidate => candidate.type === candidates[0].type) && candidates.every(candidate => (
      (candidate.type === 'toggle' && candidate.metadata?.togglePowerString)
      || (candidate.type === 'select' && String(candidate.property).toLowerCase() === 'power'
        && ['on', 'off'].every(value => candidate.options.some(option => String(option.value).toLowerCase() === value)))
    ))) {
      candidates = candidates.map(candidate => ({ ...candidate, type: 'toggle', ui: { ...candidate.ui, control: undefined }, metadata: { ...candidate.metadata, togglePowerString: true } }));
    }
    const base = { ...candidates[0] };
    if (!candidates.every((candidate) => candidate.type === base.type)) return;
    if (base.type === 'toggle' && !candidates.every(candidate => JSON.stringify(buildPayloadForInput(candidate, true)) === JSON.stringify(buildPayloadForInput(base, true)))) return;
    if (!candidates.every((candidate) => candidate.ui?.control === base.ui?.control)) {
      base.ui = { ...base.ui, control: undefined };
    }
    if (base.type === 'select') {
      const optionSets = candidates.map((candidate) => new Map(arrayOrEmpty(candidate.options).map((option) => [String(option.value).toLowerCase(), option])));
      const intersectedOptions = arrayOrEmpty(base.options).filter((option) => optionSets.every((set) => set.has(String(option.value).toLowerCase())));
      if (!intersectedOptions.length) return;
      base.options = intersectedOptions;
    }
    if (base.type === 'slider' || base.type === 'number') {
      base.range = intersectNumericRanges(candidates);
      if (!base.range) return;
    }
    sharedInputs.push(base);
    const memberValues = normalizedByDevice.map(({ device }) => getStateValueForInput(device, base));
    const numericInput = ['slider', 'number'].includes(base.type);
    const comparableValues = memberValues.filter((value) => value !== undefined && (!numericInput || value !== null && value !== ''));
    const initialValues = buildInitialControlValues(normalizedByDevice[0].device, [base]);
    const baseValue = comparableValues.length ? comparableValues[0] : initialValues[sanitizeInputKey(base)];
    sharedValues[sanitizeInputKey(base)] = base.type === 'color'
      ? extractStateColorHex(baseValue) || initialValues[sanitizeInputKey(base)]
      : base.type === 'toggle'
        ? toControlBoolean(baseValue)
        : baseValue;
    const missingNumeric = numericInput && comparableValues.length !== memberValues.length;
    if (missingNumeric || (comparableValues.length > 1 && !comparableValues.every((value) => valuesEqualByType(base.type, comparableValues[0], value)))) {
      const inputKey = sanitizeInputKey(base);
      mixedKeys.push(inputKey);
      mixedAnnotations[inputKey] = missingNumeric ? 'Some devices have no reported value' : buildMixedControlAnnotation(base, comparableValues);
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

export function buildSharedFieldCapabilities(devices, fields) {
  const members = arrayOrEmpty(devices);
  if (!members.length) return [];
  return arrayOrEmpty(fields).flatMap(field => {
    const capabilities = members.map(device => collectCapabilities(device).find(cap => cap.id === field || cap.property === field));
    const first = capabilities[0];
    const display = first?.ui?.display;
    if (!display || !capabilities.every(cap => cap && cap.ui?.display === display
      && getDisplayPresentationOptions(cap).includes(display)
      && (cap.value_type || cap.valueType) === (first.value_type || first.valueType)
      && cap.unit === first.unit
      && cap.range?.min === first.range?.min && cap.range?.max === first.range?.max)) return [];
    return [first];
  });
}