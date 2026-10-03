function isPlainObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function validateValue(value, capability) {
  const type = capability?.type;
  if (type === 'binary') return typeof value === 'boolean';
  if (type === 'numeric') {
    return Number.isFinite(value)
      && (capability.min === null || value >= capability.min)
      && (capability.max === null || value <= capability.max);
  }
  if (type === 'enum') return capability.enumValues.includes(value);
  if (type === 'string') return typeof value === 'string';
  if (type === 'object') return isPlainObject(value);
  return value === null || ['boolean', 'number', 'string'].includes(typeof value);
}

export function triggerValueFromBuilder(ui, capability) {
  let value;
  const type = capability?.type || String(ui?.value_type || 'boolean').toLowerCase();
  if (type === 'binary' || type === 'boolean') value = Boolean(ui?.value_bool);
  else if (type === 'numeric' || type === 'number') {
    const raw = String(ui?.value_number ?? '').trim();
    if (!raw) return { valid: false, message: 'Enter a number in the Builder first.' };
    value = Number(raw);
  } else value = String(ui?.value_string ?? '');

  if (!validateValue(value, capability)) return { valid: false, message: 'Builder value does not match this capability.' };
  return { valid: true, value, text: JSON.stringify(value, null, 2) };
}

export function triggerValueToBuilder(rawValue, capability) {
  let value;
  try {
    value = JSON.parse(String(rawValue || ''));
  } catch {
    return { valid: false, message: 'Value JSON is not syntactically valid.' };
  }
  if (!validateValue(value, capability)) return { valid: false, message: 'JSON value does not match this capability.' };
  const type = capability?.type;
  const ui = type === 'binary' ? { value_bool: value }
    : type === 'numeric' ? { value_number: String(value) }
      : { value_string: typeof value === 'string' ? value : '' };
  return { valid: true, value, ui };
}