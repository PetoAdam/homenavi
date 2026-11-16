export function sanitizeInputKey(input) {
  if (!input) return '';
  return input.id || input.capability_id || input.capabilityId || input.property || '';
}

export function normalizeInputType(type) {
  const normalized = String(type || '').toLowerCase();
  return normalized === 'range' ? 'slider' : normalized;
}

export function resolveInputLabel(input) {
  if (!input) return 'Control';
  if (input.label) return input.label;
  const source = input.property || input.capability_id || input.capabilityId || input.id;
  if (!source) return 'Control';
  return source
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

export function toControlBoolean(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const lowered = value.trim().toLowerCase();
    return ['on', 'true', '1', 'yes', 'enabled'].includes(lowered);
  }
  return Boolean(value);
}

const CONTROL_COMPONENTS = {
  toggle: ['toggle', 'checkbox'],
  slider: ['slider', 'number', 'stepper'],
  number: ['slider', 'number', 'stepper'],
  select: ['select', 'segmented', 'radio', 'buttons'],
  color: ['color'],
  text: ['text', 'textarea'],
};

const VALUE_TYPES = {
  toggle: ['boolean', 'bool'],
  slider: ['number', 'integer', 'float', 'double', 'int'],
  number: ['number', 'integer', 'float', 'double', 'int'],
  select: ['enum', 'string'],
  color: ['string', 'object', 'color'],
  text: ['string'],
};

export function isValidControlOverride(input) {
  const type = normalizeInputType(input?.type);
  const component = input?.ui?.control;
  if (typeof component !== 'string' || !CONTROL_COMPONENTS[type]?.includes(component)) return false;
  const valueType = String(input?.capability?.value_type || input?.capability?.valueType || '').toLowerCase();
  if (valueType && !VALUE_TYPES[type]?.includes(valueType)) return false;
  if ((type === 'slider' || type === 'number') && input.range !== undefined && input.range !== null) {
    const range = input.range;
    if (typeof range !== 'object' || Array.isArray(range)) return false;
    if (range.min !== undefined && !Number.isFinite(range.min)) return false;
    if (range.max !== undefined && !Number.isFinite(range.max)) return false;
    if (range.step !== undefined && (!Number.isFinite(range.step) || range.step <= 0)) return false;
    if (range.min !== undefined && range.max !== undefined && range.min >= range.max) return false;
    if (['integer', 'int'].includes(valueType) && [range.min, range.max, range.step].some(value => value !== undefined && !Number.isInteger(value))) return false;
  }
  if (component === 'slider' && (!Number.isFinite(input?.range?.min) || !Number.isFinite(input?.range?.max) || input.range.min >= input.range.max)) return false;
  if (['radio', 'segmented', 'buttons'].includes(component) && (!Array.isArray(input.options) || input.options.length === 0 || input.options.some(option => typeof option?.value !== 'string'))) return false;
  return true;
}

export function resolveControlComponent(input) {
  return isValidControlOverride(input) ? input.ui.control : normalizeInputType(input?.type);
}

export function getControlPresentationOptions(input) {
  return (CONTROL_COMPONENTS[normalizeInputType(input?.type)] || [])
    .filter(control => isValidControlOverride({ ...input, ui: { ...input?.ui, control } }));
}

export function resolveDisplayComponent(capability, value) {
  const component = capability?.ui?.display;
  const type = String(capability?.value_type || capability?.valueType || '').toLowerCase();
  const numeric = ['number', 'integer', 'float', 'double', 'int'].includes(type) && typeof value === 'number' && Number.isFinite(value) && (!['integer', 'int'].includes(type) || Number.isInteger(value));
  const scalar = numeric || (['boolean', 'bool'].includes(type) && typeof value === 'boolean') || (['string', 'enum'].includes(type) && typeof value === 'string');
  if (!scalar || !['metric', 'text', 'badge', 'progress', 'gauge'].includes(component)) return null;
  if (component === 'progress' || component === 'gauge') {
    const range = capability.range;
    if (!numeric || !Number.isFinite(range?.min) || !Number.isFinite(range?.max) || range.min >= range.max) return null;
  }
  return component;
}

export function getDisplayPresentationOptions(capability) {
  const type = String(capability?.value_type || capability?.valueType || '').toLowerCase();
  const sample = ['number', 'integer', 'float', 'double', 'int'].includes(type) ? 0
    : ['boolean', 'bool'].includes(type) ? true
      : ['string', 'enum'].includes(type) ? '' : null;
  if (sample === null) return [];
  return ['metric', 'text', 'badge', 'progress', 'gauge']
    .filter(display => resolveDisplayComponent({ ...capability, ui: { ...capability?.ui, display } }, sample));
}
