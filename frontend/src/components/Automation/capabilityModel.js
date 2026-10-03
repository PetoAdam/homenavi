function text(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function title(value) {
  return text(value).replace(/[_-]+/g, ' ').replace(/\b\w/g, char => char.toUpperCase());
}

function capabilityType(raw) {
  const valueType = text(raw?.value_type || raw?.valueType).toLowerCase();
  if (valueType === 'boolean') return 'binary';
  if (valueType === 'number' || valueType === 'integer') return 'numeric';
  if (valueType === 'enum') return 'enum';
  if (valueType === 'string') return 'string';
  if (valueType === 'object') return 'object';
  return 'unsupported';
}

export function normalizeCapability(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const id = text(raw.id || raw.property);
  const property = text(raw.property || raw.id);
  if (!id || !property) return null;
  const range = raw.range && typeof raw.range === 'object' ? raw.range : {};
  const access = raw.access && typeof raw.access === 'object' ? raw.access : {};
  return {
    id,
    property,
    label: text(raw.name) || title(property),
    type: capabilityType(raw),
    readable: access.read !== false,
    writable: access.write === true,
    unit: text(raw.unit),
    min: Number.isFinite(Number(range.min)) ? Number(range.min) : null,
    max: Number.isFinite(Number(range.max)) ? Number(range.max) : null,
    step: Number.isFinite(Number(range.step)) && Number(range.step) > 0 ? Number(range.step) : null,
    enumValues: Array.isArray(raw.enum) ? raw.enum.map(text).filter(Boolean) : [],
    raw,
  };
}

export function capabilitiesForDevice(device) {
  const capabilities = Array.isArray(device?.capabilities) ? device.capabilities : [];
  return capabilities.map(normalizeCapability).filter(Boolean);
}

export function compatibleCapabilities(devices, { writable = false } = {}) {
  const sets = (Array.isArray(devices) ? devices : []).map(capabilitiesForDevice);
  if (sets.length === 0) return [];
  return sets[0].filter(capability => {
    if (writable && !capability.writable) return false;
    return sets.slice(1).every(set => set.some(candidate => (
      candidate.id === capability.id
      && candidate.property === capability.property
      && candidate.type === capability.type
      && (!writable || candidate.writable)
    )));
  });
}

export function operatorsForCapability(capability) {
  if (!capability) return ['exists', 'changed'];
  if (capability.type === 'numeric') return ['exists', 'changed', 'eq', 'neq', 'gt', 'gte', 'lt', 'lte'];
  if (capability.type === 'binary' || capability.type === 'enum') return ['exists', 'changed', 'eq', 'neq'];
  return ['exists', 'changed'];
}