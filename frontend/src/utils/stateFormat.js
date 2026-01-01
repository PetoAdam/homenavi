// Shared state formatting helpers used across Map and Device UI.

export function coerceBooleanLike(raw) {
  if (raw === undefined || raw === null || raw === '') return undefined;

  if (typeof raw === 'boolean') return raw;
  // Many sensors report numeric readings (temperature, humidity, etc.).
  // Only treat numbers as boolean-like when they are clearly binary.
  if (typeof raw === 'number') {
    if (raw === 0) return false;
    if (raw === 1) return true;
    return undefined;
  }

  if (typeof raw === 'string') {
    const lowered = raw.trim().toLowerCase();
    if (['true', 'on', '1', 'yes', 'enabled', 'detected', 'active', 'closed', 'present'].includes(lowered)) {
      return true;
    }
    if (['false', 'off', '0', 'no', 'disabled', 'clear', 'inactive', 'open', 'absent'].includes(lowered)) {
      return false;
    }
  }

  return undefined;
}

export function normalizeQuickValue(value) {
  if (value === undefined || value === null || value === '') return '';
  if (typeof value === 'boolean') return value ? 'On' : 'Off';

  if (typeof value === 'number') {
    if (Number.isInteger(value)) return String(value);
    return String(Math.round(value * 100) / 100);
  }

  if (typeof value === 'string') {
    const lowered = value.trim().toLowerCase();
    if (['true', 'on', '1', 'yes', 'enabled', 'detected', 'active', 'closed', 'present'].includes(lowered)) return 'On';
    if (['false', 'off', '0', 'no', 'disabled', 'clear', 'inactive', 'open', 'absent'].includes(lowered)) return 'Off';
    return value;
  }

  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return '';
    }
  }

  return String(value);
}

export function formatBinaryStateValue(key, raw) {
  const bool = coerceBooleanLike(raw);
  if (bool === undefined) return '—';

  const normalizedKey = (key || '').toString().trim().toLowerCase();
  switch (normalizedKey) {
  case 'contact':
    return bool ? 'Closed' : 'Open';
  case 'open':
    return bool ? 'Open' : 'Closed';
  case 'closed':
    return bool ? 'Closed' : 'Open';
  case 'occupancy':
  case 'motion':
  case 'presence':
    return bool ? 'Detected' : 'Clear';
  case 'water_leak':
  case 'leak':
  case 'moisture':
    return bool ? 'Leak' : 'Dry';
  case 'smoke':
    return bool ? 'Smoke' : 'Clear';
  case 'tamper':
    return bool ? 'Tamper' : 'Secure';
  case 'battery_low':
  case 'low_battery':
    return bool ? 'Low' : 'OK';
  default:
    return bool ? 'On' : 'Off';
  }
}

export function formatStateValueForKey(key, raw) {
  const k = (key || '').toString().trim().toLowerCase();
  if (!k) return normalizeQuickValue(raw);

  // Prefer binary mappings (Open/Closed, Detected/Clear, etc.) when value is boolean-like.
  const bool = coerceBooleanLike(raw);
  if (bool !== undefined) {
    // Keep "—" behavior consistent with device cards.
    return formatBinaryStateValue(k, raw);
  }

  if (raw === undefined || raw === null || raw === '') return '';

  if (typeof raw === 'number') {
    if (k.includes('battery')) return `${Math.round(raw)}%`;
    if (k.includes('temp')) return `${Math.round(raw * 10) / 10}`;
    if (k.includes('humid')) return `${Math.round(raw)}%`;
  }

  if (typeof raw === 'string') {
    const lowered = raw.trim().toLowerCase();
    if (lowered === 'open') return 'Open';
    if (lowered === 'closed') return 'Closed';
    if (lowered === 'on') return 'On';
    if (lowered === 'off') return 'Off';
  }

  return normalizeQuickValue(raw);
}

function formatNumberLikeGlassMetric(value) {
  if (!Number.isFinite(value)) return '';
  const abs = Math.abs(value);
  const formatted = abs >= 100 ? value.toFixed(0) : value.toFixed(1);
  return formatted.replace(/\.0$/, '');
}

function normalizeKeyForMatch(key) {
  return (key || '')
    .toString()
    .trim()
    .toLowerCase();
}

function keyVariants(key) {
  const k = normalizeKeyForMatch(key);
  if (!k) return [];
  const noSep = k.replace(/[_\-\s]+/g, '');
  const camel = k.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
  return Array.from(new Set([k, noSep, camel.toLowerCase()]));
}

function findCapabilityForKey(key, capabilities) {
  const caps = Array.isArray(capabilities) ? capabilities : [];
  const variants = new Set(keyVariants(key));
  if (variants.size === 0) return null;

  for (const cap of caps) {
    if (!cap || typeof cap !== 'object') continue;
    const candidates = [cap.id, cap.property, cap.name]
      .filter(Boolean)
      .map(v => v.toString());
    for (const candidate of candidates) {
      const cVariants = keyVariants(candidate);
      if (cVariants.some(v => variants.has(v))) {
        return cap;
      }
    }
  }
  return null;
}

export function inferUnitForKey(key, capabilities) {
  const k = normalizeKeyForMatch(key);
  if (!k) return '';

  const cap = findCapabilityForKey(k, capabilities);
  if (cap) {
    return (cap.unit || cap.units || '').toString();
  }

  // Fallbacks (mirror what device cards do when capability metadata is missing).
  if (k.includes('temp')) return '°C';
  if (k.includes('humid')) return '%';
  if (k.includes('battery')) return '%';
  return '';
}

function coerceNumberLike(raw) {
  if (typeof raw === 'number') return raw;
  if (typeof raw !== 'string') return undefined;
  const s = raw.trim();
  if (!s) return undefined;
  // Accept only plain numeric strings.
  if (!/^-?\d+(?:\.\d+)?$/.test(s)) return undefined;
  const n = Number(s);
  return Number.isFinite(n) ? n : undefined;
}

export function formatMetricValueAndUnitForKey(key, raw, capabilities) {
  const k = normalizeKeyForMatch(key);
  if (!k) {
    return { valueText: normalizeQuickValue(raw), unit: '' };
  }

  // Prefer Device Hub capability metadata when available.
  const cap = findCapabilityForKey(k, capabilities);
  if (cap) {
    const kind = (cap.kind || cap.type || '').toString().trim().toLowerCase();
    const valueType = (cap.value_type || cap.valueType || '').toString().trim().toLowerCase();
    const capKey = cap.property || cap.id || key;

    if (kind === 'binary' || valueType === 'boolean') {
      return { valueText: formatBinaryStateValue(capKey, raw), unit: '' };
    }

    // Numeric capabilities should stay numeric even if the raw looks truthy.
    if (kind === 'numeric' || valueType === 'number' || valueType === 'float' || valueType === 'double' || valueType === 'integer' || valueType === 'int') {
      const n = coerceNumberLike(raw);
      return {
        valueText: n !== undefined ? formatNumberLikeGlassMetric(n) : normalizeQuickValue(raw),
        unit: (cap.unit || cap.units || inferUnitForKey(capKey, capabilities) || '').toString(),
      };
    }
  }

  // Binary states: never show units.
  const bool = coerceBooleanLike(raw);
  if (bool !== undefined) {
    return { valueText: formatBinaryStateValue(k, raw), unit: '' };
  }

  if (raw === undefined || raw === null || raw === '') {
    return { valueText: '', unit: '' };
  }

  const n = coerceNumberLike(raw);
  if (n !== undefined) {
    return {
      valueText: formatNumberLikeGlassMetric(n),
      unit: inferUnitForKey(k, capabilities),
    };
  }

  return { valueText: normalizeQuickValue(raw), unit: '' };
}
