function isPlainObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function isCapabilityValueValid(value, capability) {
  if (capability.type === 'binary') return typeof value === 'boolean';
  if (capability.type === 'numeric') {
    if (!Number.isFinite(value)) return false;
    return (capability.min === null || value >= capability.min)
      && (capability.max === null || value <= capability.max);
  }
  if (capability.type === 'enum') return capability.enumValues.includes(value);
  if (capability.type === 'string') return typeof value === 'string';
  if (capability.type === 'object') return isPlainObject(value);
  return false;
}

export function buildArgsFromBuilder(data) {
  const commands = Array.isArray(data?.capability_commands) && data.capability_commands.length > 0
    ? data.capability_commands
    : [{ id: data?.capability_id, property: data?.capability_property || data?.capability_id, value: data?.capability_value }];
  const args = {};
  for (const command of commands) {
    const property = String(command?.property || command?.id || '').trim();
    if (!property || typeof command?.value === 'undefined' || Object.prototype.hasOwnProperty.call(args, property)) {
      return { valid: false, message: 'Each Builder capability needs a unique value.' };
    }
    args[property] = command.value;
  }
  if (Object.keys(args).length === 0) {
    return { valid: false, message: 'Select a capability and value in the Builder first.' };
  }
  return { valid: true, args };
}

export function validateCommandArgs(rawArgs, { commandMode = 'set_state', writableCapabilities = [] } = {}) {
  let args;
  try {
    args = JSON.parse(String(rawArgs || '{}'));
  } catch {
    return { valid: false, message: 'JSON is not syntactically valid.' };
  }

  if (!isPlainObject(args)) return { valid: false, message: 'Command args must be a JSON object.' };
  if (commandMode === 'custom') return { valid: true, message: 'JSON is valid for this custom command.', args };

  const entries = Object.entries(args);
  if (entries.length === 0) return { valid: false, message: 'Set state JSON must contain at least one capability property.' };
  const capabilities = [];
  for (const [property, value] of entries) {
    const capability = writableCapabilities.find(item => item.property === property);
    if (!capability) return { valid: false, message: `“${property}” is not writable for this target.` };
    if (!isCapabilityValueValid(value, capability)) return { valid: false, message: `Value for ${capability.label} does not match its capability type or range.` };
    capabilities.push(capability);
  }
  return { valid: true, message: `${capabilities.length} capability value${capabilities.length === 1 ? '' : 's'} are valid for this target.`, args, capabilities, capability: capabilities[0] };
}