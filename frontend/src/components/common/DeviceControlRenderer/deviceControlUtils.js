export function sanitizeInputKey(input) {
  if (!input) return '';
  return input.id || input.capability_id || input.capabilityId || input.property || '';
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
