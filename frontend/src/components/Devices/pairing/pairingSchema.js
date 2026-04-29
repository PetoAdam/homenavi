const DEFAULT_PAIRING_FLOW_STEPS = [
  {
    id: 'pairing_active',
    label: 'Pairing Active',
    description: 'Adapter is listening for candidate devices.',
    statuses: ['starting', 'active', 'in_progress'],
  },
  {
    id: 'pairing_detected',
    label: 'Device Detected',
    description: 'A candidate device has been discovered.',
    statuses: ['device_detected', 'device_joined'],
  },
  {
    id: 'pairing_finalize',
    label: 'Finalizing',
    description: 'Applying metadata and completing registration.',
    statuses: ['interviewing', 'interview_complete'],
  },
  {
    id: 'pairing_completed',
    label: 'Completed',
    description: 'Pairing workflow completed successfully.',
    statuses: ['completed'],
  },
];

function toArray(value) {
  if (Array.isArray(value)) return value;
  return [];
}

function normalizeField(field, index = 0) {
  if (!field || typeof field !== 'object') return null;
  const key = `${field.id || field.key || `field-${index + 1}`}`.trim();
  if (!key) return null;
  const component = `${field.component || field.type || 'text'}`.trim().toLowerCase();
  return {
    id: key,
    component,
    type: `${field.type || component || 'text'}`.trim().toLowerCase(),
    bind: `${field.bind || ''}`.trim().toLowerCase(),
    label: `${field.label || key}`,
    description: `${field.description || ''}`,
    placeholder: `${field.placeholder || ''}`,
    required: Boolean(field.required),
    options: toArray(field.options).map(option => {
      if (option && typeof option === 'object') {
        const value = option.value ?? option.id ?? option.key ?? option.label;
        const label = option.label ?? option.name ?? option.value ?? option.id ?? option.key;
        return {
          value: `${value ?? ''}`,
          label: `${label ?? value ?? ''}`,
          description: `${option.description || ''}`,
        };
      }
      return { value: `${option ?? ''}`, label: `${option ?? ''}`, description: '' };
    }).filter(option => option.value),
    multiple: Boolean(field.multiple),
    min: Number.isFinite(Number(field.min)) ? Number(field.min) : null,
    max: Number.isFinite(Number(field.max)) ? Number(field.max) : null,
    step: Number.isFinite(Number(field.step)) ? Number(field.step) : null,
    defaultValue: field.default ?? field.defaultValue ?? '',
    mode: `${field.mode || ''}`.trim().toLowerCase(),
    metadata: field.metadata && typeof field.metadata === 'object' ? { ...field.metadata } : {},
  };
}

function normalizeForm(form, index = 0) {
  if (!form || typeof form !== 'object') return null;
  const mode = `${form.mode || form.id || `mode-${index + 1}`}`.trim().toLowerCase();
  const fields = toArray(form.fields).map((field, fieldIndex) => normalizeField(field, fieldIndex)).filter(Boolean);
  return {
    id: `${form.id || mode || `form-${index + 1}`}`,
    mode,
    label: `${form.label || mode || `Mode ${index + 1}`}`,
    description: `${form.description || ''}`,
    fields,
  };
}

export function normalizePairingFlow(flow) {
  if (!flow || typeof flow !== 'object') {
    return {
      steps: DEFAULT_PAIRING_FLOW_STEPS,
      hints: [],
      forms: [],
      entryModes: ['default'],
      flowId: '',
    };
  }

  const rawSteps = toArray(flow.steps);
  const steps = rawSteps
    .map((step, index) => {
      if (!step || typeof step !== 'object') return null;
      const statuses = [
        ...toArray(step.statuses),
        ...toArray(step.matches),
        ...toArray(step.stages),
      ]
        .map(item => `${item || ''}`.trim().toLowerCase())
        .filter(Boolean);
      const stage = `${step.stage || ''}`.trim().toLowerCase();
      if (stage && !statuses.includes(stage)) {
        statuses.push(stage);
      }
      return {
        id: `${step.id || `step-${index + 1}`}`,
        label: `${step.label || `Step ${index + 1}`}`,
        description: `${step.description || ''}`,
        statuses,
      };
    })
    .filter(Boolean);

  const forms = toArray(flow.forms)
    .map((form, index) => normalizeForm(form, index))
    .filter(Boolean);

  const entryModes = toArray(flow.entry_modes)
    .map(item => `${item || ''}`.trim().toLowerCase())
    .filter(Boolean);

  if (entryModes.length === 0) {
    if (forms.length > 0) {
      forms.forEach(form => {
        if (form.mode && !entryModes.includes(form.mode)) {
          entryModes.push(form.mode);
        }
      });
    }
  }

  return {
    steps: steps.length > 0 ? steps : DEFAULT_PAIRING_FLOW_STEPS,
    hints: toArray(flow.hints).map(item => `${item || ''}`.trim()).filter(Boolean),
    forms,
    entryModes: entryModes.length > 0 ? entryModes : ['default'],
    flowId: `${flow.id || flow.flow_id || ''}`.trim(),
  };
}

export function resolveCurrentPairingStep(status, flow) {
  const pairingFlow = normalizePairingFlow(flow);
  const steps = pairingFlow.steps;
  const normalized = `${status || ''}`.toLowerCase();

  const activeIndex = (() => {
    if (!normalized) return 0;
    const idx = steps.findIndex(phase => phase.statuses.includes(normalized));
    if (idx >= 0) return idx;
    if (normalized === 'completed') return steps.length - 1;
    return 0;
  })();

  return {
    index: activeIndex,
    step: steps[activeIndex] || null,
    total: steps.length,
  };
}

export function buildPairingPhaseStates(status, flow) {
  const pairingFlow = normalizePairingFlow(flow);
  const steps = pairingFlow.steps;
  const { index: activeIndex } = resolveCurrentPairingStep(status, flow);

  return steps.map((phase, index) => {
    let state = 'upcoming';
    if (activeIndex > index) state = 'complete';
    else if (activeIndex === index) state = 'active';
    return { ...phase, state };
  });
}

export function getPairingHints(pairingProfile) {
  const instructions = pairingProfile?.instructions;
  if (Array.isArray(instructions) && instructions.length > 0) {
    return instructions;
  }
  const flow = normalizePairingFlow(pairingProfile?.flow);
  return flow.hints;
}

export function getPairingProfile(protocol, config) {
  const key = `${protocol || ''}`.trim().toLowerCase();
  if (!key) return null;
  return config?.[key] || null;
}

export function getProtocolOptions(integrations = [], pairingConfig = {}) {
  const integrationMeta = new Map();
  integrations.forEach(item => {
    if (!item || !item.protocol) return;
    const key = `${item.protocol}`.trim().toLowerCase();
    if (!key) return;
    integrationMeta.set(key, {
      label: item.label || item.protocol,
      status: item.status || '',
    });
  });

  const options = [];
  Object.values(pairingConfig || {}).forEach(item => {
    if (!item || !item.protocol) return;
    const key = `${item.protocol}`.trim().toLowerCase();
    if (!key) return;

    const schemaVersion = `${item.schema_version || ''}`.trim();
    const supported = Boolean(item.supported);
    if (!schemaVersion || !supported) return;

    const integration = integrationMeta.get(key);
    if (integration?.status?.toLowerCase() === 'planned') return;

    options.push({
      protocol: key,
      label: integration?.label || item.label || item.protocol,
      status: 'active',
    });
  });

  options.sort((a, b) => `${a.label}`.localeCompare(`${b.label}`, undefined, { sensitivity: 'base' }));
  return options;
}

export function isPairingSupported(pairingProfile, integrationStatus = '') {
  const blocked = `${integrationStatus || ''}`.toLowerCase() === 'planned';
  const hasSchemaVersion = typeof pairingProfile?.schema_version === 'string' && pairingProfile.schema_version.trim().length > 0;
  return Boolean(pairingProfile?.supported) && hasSchemaVersion && !blocked;
}

export function getBlockedReason(pairingSupported, activeSessionForSelected, pairingProfile) {
  if (pairingSupported) {
    return activeSessionForSelected?.active ? 'Pairing already running for this protocol.' : '';
  }
  if (!pairingProfile) {
    return 'Adapter has not published pairing schema yet.';
  }
  if (typeof pairingProfile?.schema_version !== 'string' || !pairingProfile.schema_version.trim()) {
    return 'Adapter pairing schema_version is missing.';
  }
  if (pairingProfile?.notes) {
    return pairingProfile.notes;
  }
  return 'Guided pairing is not available for this protocol yet.';
}

export function getFieldsForMode(flow, mode) {
  const normalized = normalizePairingFlow(flow);
  if (normalized.forms.length === 0) {
    return [];
  }
  const modeKey = `${mode || ''}`.trim().toLowerCase();
  const byMode = normalized.forms.find(form => form.mode === modeKey);
  if (byMode) return byMode.fields;
  return normalized.forms[0]?.fields || [];
}

export function buildPairingStartPayload({
  protocol,
  pairingProfile,
  selectedMode,
  fieldValues,
}) {
  const normalizedProtocol = `${protocol || ''}`.trim().toLowerCase();
  const flow = normalizePairingFlow(pairingProfile?.flow);
  const fields = getFieldsForMode(pairingProfile?.flow, selectedMode);
  const inputs = {};
  let timeout = Number(pairingProfile?.default_timeout_sec || pairingProfile?.defaultTimeoutSec || 180);

  fields.forEach(field => {
    const value = fieldValues?.[field.id];
    if (typeof value === 'undefined' || value === null || value === '') return;
    if (field.bind === 'timeout' || field.id === 'timeout') {
      const parsed = Number(value);
      if (Number.isFinite(parsed) && parsed > 0) {
        timeout = parsed;
      }
      return;
    }
    inputs[field.id] = value;
  });

  return {
    protocol: normalizedProtocol,
    timeout,
    mode: `${selectedMode || ''}`.trim().toLowerCase() || flow.entryModes[0] || 'default',
    flow_id: flow.flowId || undefined,
    inputs,
  };
}

export function defaultFieldValues(flow, mode) {
  const fields = getFieldsForMode(flow, mode);
  const values = {};
  fields.forEach(field => {
    if (typeof field.defaultValue !== 'undefined') {
      values[field.id] = field.defaultValue;
      return;
    }
    if (field.component === 'checkbox') {
      values[field.id] = false;
      return;
    }
    if (field.multiple || field.component === 'multiselect') {
      values[field.id] = [];
      return;
    }
    values[field.id] = '';
  });
  return values;
}

function modeHasField(normalizedFlow, mode, fieldId) {
  const modeKey = `${mode || ''}`.trim().toLowerCase();
  const target = `${fieldId || ''}`.trim();
  if (!modeKey || !target) return false;
  const form = normalizedFlow.forms.find(item => item.mode === modeKey);
  if (!form) return false;
  return form.fields.some(field => field.id === target);
}

function chooseModeWithField(normalizedFlow, preferredModes, fieldId) {
  const modes = Array.isArray(preferredModes) ? preferredModes : [];
  for (const mode of modes) {
    if (modeHasField(normalizedFlow, mode, fieldId)) {
      return `${mode}`.trim().toLowerCase();
    }
  }
  const form = normalizedFlow.forms.find(item => item.fields.some(field => field.id === fieldId));
  return form?.mode || '';
}

function chooseModeForRequiredInputs(normalizedFlow, requiredInputs, fallbackMode) {
  const targets = Array.isArray(requiredInputs) ? requiredInputs.map(item => `${item || ''}`.trim()).filter(Boolean) : [];
  if (targets.length === 0) return fallbackMode;
  if (modeHasField(normalizedFlow, fallbackMode, targets[0])) {
    return fallbackMode;
  }

  const scored = normalizedFlow.forms
    .map(form => {
      const fieldIds = new Set(form.fields.map(field => field.id));
      const score = targets.reduce((acc, item) => (fieldIds.has(item) ? acc + 1 : acc), 0);
      return { mode: form.mode, score };
    })
    .sort((a, b) => b.score - a.score);

  if (scored.length > 0 && scored[0].score > 0) {
    return scored[0].mode;
  }
  return fallbackMode;
}

export function deriveRecoveryPreset({
  pairingProfile,
  currentMode,
  currentValues,
  requiredFieldIds,
  terminalStatus,
  errorCode,
  preferAlternateMode = false,
}) {
  const normalizedFlow = normalizePairingFlow(pairingProfile?.flow);
  const entryModes = normalizedFlow.entryModes;
  const fallbackMode = `${currentMode || entryModes[0] || 'default'}`.trim().toLowerCase() || 'default';
  const baseValues = currentValues && typeof currentValues === 'object' ? { ...currentValues } : {};
  const requiredInputs = Array.isArray(requiredFieldIds) ? requiredFieldIds.map(item => `${item || ''}`.trim()).filter(Boolean) : [];
  const status = `${terminalStatus || ''}`.trim().toLowerCase();

  let nextMode = chooseModeForRequiredInputs(normalizedFlow, requiredInputs, fallbackMode);

  void errorCode;

  if (preferAlternateMode && entryModes.length > 1) {
    const currentKey = `${currentMode || ''}`.trim().toLowerCase();
    const modeByPreset = `${nextMode || ''}`.trim().toLowerCase();
    const alternative = entryModes.find(mode => mode !== modeByPreset && mode !== currentKey)
      || entryModes.find(mode => mode !== currentKey)
      || entryModes[0];
    if (alternative) {
      nextMode = alternative;
    }
  }

  if (!nextMode) {
    nextMode = fallbackMode;
  }

  if ((status === 'timeout' || status === 'stopped') && typeof baseValues.network_path === 'string' && baseValues.network_path.trim() === '') {
    delete baseValues.network_path;
  }

  return {
    mode: nextMode,
    values: baseValues,
    requiredFieldIds: requiredInputs,
  };
}
