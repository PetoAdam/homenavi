import React, { useEffect, useMemo, useRef, useState } from 'react';
import PairingFieldRenderer from './PairingFieldRenderer';
import {
  buildPairingStartPayload,
  defaultFieldValues,
  getFieldsForMode,
  normalizePairingFlow,
} from './pairingSchema';

function modeLabel(mode) {
  if (!mode) return 'Default';
  return mode.replace(/[_-]+/g, ' ').replace(/\b\w/g, char => char.toUpperCase());
}

function hasValue(value) {
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return Number.isFinite(value);
  return `${value ?? ''}`.trim().length > 0;
}

function requiredFieldsMissing(fields, values) {
  return fields.some(field => field.required && !hasValue(values[field.id]));
}

function humanizeKey(value) {
  return `${value || ''}`
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, char => char.toUpperCase())
    .trim();
}

function buildModeOptions(entryModes, flowForms) {
  return entryModes.map(mode => {
    const form = flowForms.find(item => item.mode === mode);
    return {
      mode,
      label: form?.label || modeLabel(mode),
      description: form?.description || '',
    };
  });
}

export default function PairingFlowRenderer({
  protocol,
  pairingProfile,
  disabled,
  busy,
  ctaLabel,
  blockedReason,
  onStart,
  onError,
  initialMode = '',
  initialValues = null,
  requiredFieldIds = [],
}) {
  const normalizedFlow = useMemo(() => normalizePairingFlow(pairingProfile?.flow), [pairingProfile]);
  const entryModes = normalizedFlow.entryModes;
  const flowForms = normalizedFlow.forms;
  const modeOptions = useMemo(() => buildModeOptions(entryModes, flowForms), [entryModes, flowForms]);
  const shouldUseGuidedWizard = entryModes.length > 1;
  const [selectedMode, setSelectedMode] = useState(initialMode || entryModes[0] || 'default');
  const [values, setValues] = useState(() => {
    const base = defaultFieldValues(pairingProfile?.flow, initialMode || entryModes[0] || 'default');
    return initialValues && typeof initialValues === 'object' ? { ...base, ...initialValues } : base;
  });
  const [guidedStep, setGuidedStep] = useState(() => (entryModes.length > 1 ? 'mode' : 'inputs'));

  // Stable serialized versions of array/object props so the init effect only
  // fires when values meaningfully change, not on every parent re-render.
  const requiredFieldIdsKey = Array.isArray(requiredFieldIds) ? requiredFieldIds.join(',') : '';
  const initialModeKey = initialMode || '';
  // We track whether we've already applied the initial reset for this "session"
  // of the component so that once the user advances past 'mode' we never pull
  // them back due to a parent re-render.
  const initKeyRef = useRef(null);

  useEffect(() => {
    const newKey = `${initialModeKey}|${requiredFieldIdsKey}`;
    // Only fully re-initialize when the controlling identity changes.
    if (initKeyRef.current === newKey) return;
    initKeyRef.current = newKey;

    const nextMode = initialModeKey || entryModes[0] || 'default';
    setSelectedMode(nextMode);
    const base = defaultFieldValues(pairingProfile?.flow, nextMode);
    setValues(initialValues && typeof initialValues === 'object' ? { ...base, ...initialValues } : base);
    if (requiredFieldIdsKey.length > 0) {
      setGuidedStep('inputs');
      return;
    }
    setGuidedStep(entryModes.length > 1 ? 'mode' : 'inputs');
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialModeKey, requiredFieldIdsKey]);

  const fields = useMemo(() => getFieldsForMode(pairingProfile?.flow, selectedMode), [pairingProfile, selectedMode]);
  const selectedForm = useMemo(
    () => flowForms.find(form => form.mode === selectedMode) || null,
    [flowForms, selectedMode],
  );
  const guidanceHints = useMemo(
    () => (Array.isArray(pairingProfile?.instructions) ? pairingProfile.instructions.filter(Boolean) : []),
    [pairingProfile],
  );
  const filteredFields = useMemo(() => {
    if (!Array.isArray(requiredFieldIds) || requiredFieldIds.length === 0) {
      return fields;
    }
    const allowed = new Set(requiredFieldIds.map(item => `${item}`.trim()).filter(Boolean));
    const requiredOnly = fields.filter(field => allowed.has(field.id));
    return requiredOnly.length > 0 ? requiredOnly : fields;
  }, [fields, requiredFieldIds]);
  const requiredInputsMissing = useMemo(
    () => requiredFieldsMissing(filteredFields, values),
    [filteredFields, values],
  );
  const guidedSteps = useMemo(() => {
    const steps = [];
    if (entryModes.length > 1) {
      steps.push({ id: 'mode', label: 'Select mode' });
    }
    steps.push({ id: 'inputs', label: 'Prepare device' });
    steps.push({ id: 'review', label: 'Start pairing' });
    return steps;
  }, [entryModes.length]);
  const guidedStepIndex = guidedSteps.findIndex(step => step.id === guidedStep);
  const fieldLabelById = useMemo(() => {
    const map = new Map();
    flowForms.forEach(form => {
      form.fields.forEach(field => {
        if (!map.has(field.id)) {
          map.set(field.id, field.label || humanizeKey(field.id));
        }
      });
    });
    return map;
  }, [flowForms]);
  const stepContext = useMemo(() => {
    if (guidedStep === 'mode') {
      return {
        title: 'Choose pairing mode',
        description: 'Pick how you want to onboard the device. You can switch this before starting.',
      };
    }
    if (guidedStep === 'review') {
      return {
        title: 'Review and start',
        description: 'Confirm entered values, then start pairing. Live progress appears immediately after start.',
      };
    }
    return {
      title: Array.isArray(requiredFieldIds) && requiredFieldIds.length > 0
        ? 'Provide missing inputs'
        : 'Prepare device and inputs',
      description: 'Follow adapter instructions and complete required fields before continuing.',
    };
  }, [guidedStep, requiredFieldIds]);

  const handleFieldChange = (fieldId, value) => {
    setValues(prev => ({ ...prev, [fieldId]: value }));
  };

  const handleModeChange = event => {
    const mode = event.target.value;
    setSelectedMode(mode);
    setValues(defaultFieldValues(pairingProfile?.flow, mode));
  };

  const handleNext = () => {
    if (guidedStep === 'mode') {
      setGuidedStep('inputs');
      return;
    }
    if (guidedStep === 'inputs') {
      if (requiredInputsMissing) {
        onError?.('Please fill all required fields before continuing.');
        return;
      }
      setGuidedStep('review');
    }
  };

  const handleBack = () => {
    if (guidedStep === 'review') {
      setGuidedStep('inputs');
      return;
    }
    if (guidedStep === 'inputs' && entryModes.length > 1) {
      setGuidedStep('mode');
    }
  };

  const handleStart = async () => {
    if (disabled || busy || typeof onStart !== 'function') {
      return;
    }
    try {
      const payload = buildPairingStartPayload({
        protocol,
        pairingProfile,
        selectedMode,
        fieldValues: values,
      });
      await onStart(payload);
    } catch (error) {
      if (typeof onError === 'function') {
        onError(error?.message || 'Unable to start pairing');
      }
    }
  };

  return (
    <div className="add-device-guided-actions add-device-guided-schema-renderer">
      {shouldUseGuidedWizard ? (
        <div className="add-device-pairing-step-header">
          <span className="add-device-pairing-step-kicker">Step {Math.max(guidedStepIndex + 1, 1)} of {guidedSteps.length}</span>
          <h5>{stepContext.title}</h5>
          <p>{stepContext.description}</p>
        </div>
      ) : null}

      {shouldUseGuidedWizard ? (
        <div className="add-device-pairing-guided-steps" role="list" aria-label="Pairing setup steps">
          {guidedSteps.map((step, index) => {
            let state = 'upcoming';
            if (guidedStepIndex > index) state = 'complete';
            if (guidedStepIndex === index) state = 'active';
            return (
              <div key={step.id} role="listitem" className={`add-device-pairing-guided-step ${state}`}>
                <span>{index + 1}</span>
                <strong>{step.label}</strong>
              </div>
            );
          })}
        </div>
      ) : null}

      {(guidedStep === 'mode' || !shouldUseGuidedWizard) && entryModes.length > 1 && requiredFieldIds.length === 0 ? (
        <div className="add-device-pairing-field">
          <span className="add-device-pairing-field-label">Pairing mode</span>
          <div className="add-device-pairing-selector cards" role="list" aria-label="Pairing mode">
            {modeOptions.map(option => (
              <div key={option.mode} role="listitem">
                <button
                  type="button"
                  className={`add-device-pairing-selector-item${selectedMode === option.mode ? ' active' : ''}`}
                  onClick={() => handleModeChange({ target: { value: option.mode } })}
                >
                  <strong>{option.label}</strong>
                  {option.description ? <span>{option.description}</span> : null}
                </button>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {(guidedStep === 'inputs' || !shouldUseGuidedWizard) && filteredFields.length > 0 ? (
        <div className="add-device-pairing-fields-grid">
          {filteredFields.map(field => (
            <PairingFieldRenderer
              key={field.id}
              field={field}
              value={values[field.id]}
              onChange={handleFieldChange}
            />
          ))}
        </div>
      ) : (guidedStep === 'inputs' || !shouldUseGuidedWizard) ? (
        <small className="add-device-modal-hint">No adapter-defined form fields for this mode.</small>
      ) : null}

      {(guidedStep === 'inputs' || guidedStep === 'review') && selectedForm?.description ? (
        <div className="add-device-pairing-text-block">{selectedForm.description}</div>
      ) : null}

      {(guidedStep === 'inputs' || guidedStep === 'review') && guidanceHints.length > 0 ? (
        <ul className="add-device-pairing-guidance-list">
          {guidanceHints.map(hint => <li key={hint}>{hint}</li>)}
        </ul>
      ) : null}

      {guidedStep === 'review' ? (
        <div className="add-device-pairing-review">
          <div>
            <strong>Mode</strong>
            <span>{modeLabel(selectedMode)}</span>
          </div>
          {Object.entries(values)
            .filter(([, value]) => hasValue(value))
            .map(([key, value]) => (
              <div key={key}>
                <strong>{fieldLabelById.get(key) || humanizeKey(key)}</strong>
                <span>{Array.isArray(value) ? value.join(', ') : `${value}`}</span>
              </div>
            ))}
        </div>
      ) : null}

      {shouldUseGuidedWizard ? (
        <div className="add-device-pairing-guided-actions">
          <button
            type="button"
            className="add-device-secondary-btn"
            onClick={handleBack}
            disabled={guidedStep === guidedSteps[0]?.id}
          >
            Back
          </button>
          {guidedStep === 'review' ? (
            <button
              type="button"
              className="auth-modal-btn add-device-guided-action"
              onClick={handleStart}
              disabled={disabled || busy}
            >
              {busy ? 'Starting…' : ctaLabel}
            </button>
          ) : (
            <button
              type="button"
              className="auth-modal-btn add-device-guided-action"
              onClick={handleNext}
              disabled={disabled || busy || (guidedStep === 'inputs' && requiredInputsMissing)}
            >
              Continue
            </button>
          )}
        </div>
      ) : (
        <button
          type="button"
          className="auth-modal-btn add-device-guided-action"
          onClick={handleStart}
          disabled={disabled || busy}
        >
          {busy ? 'Starting…' : ctaLabel}
        </button>
      )}

      <small className="add-device-modal-hint">
        {blockedReason || 'Pairing fields are rendered from adapter schema.'}
      </small>
    </div>
  );
}
