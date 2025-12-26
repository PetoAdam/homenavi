import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChevronDown, faCircleCheck } from '@fortawesome/free-solid-svg-icons';
import { DEVICE_ICON_CHOICES } from './deviceIconChoices';
import { getModalRoot } from '../common/Modal/modalRoot';
import './AddDeviceModal.css';

const CAPABILITIES_EXAMPLE = '[{ "id": "state", "kind": "binary" }]';
const INPUTS_EXAMPLE = '[{ "type": "toggle", "property": "state" }]';
const PAIRING_INSTRUCTIONS = {
  zigbee: [
    'Reset or power-cycle the device to enter pairing mode.',
    'Keep it close to the Zigbee coordinator while pairing runs.',
    'Wait here—we will auto-register it as soon as it is detected.',
  ],
  thread: [
    'Make sure the Thread border router is online.',
    'Put the Thread device into commissioning mode.',
    'We will record the session as soon as the adapter reports it.',
  ],
  matter: [
    'Open the Matter device commissioning window.',
    'Keep the QR code handy in case an app flow is required.',
    'We will attach the device when the hub sees it join.',
  ],
};

const DEFAULT_PAIRING_INSTRUCTIONS = [
  'Power-cycle or reset the device to start discovery.',
  'Keep it close to the hub while pairing runs.',
  'We will register it automatically when detected.',
];

const FLOW_STEPS = [
  { id: 'form', label: 'Details' },
  { id: 'pairing', label: 'Pairing' },
  { id: 'success', label: 'Complete' },
];

const PAIRING_PHASES = [
  {
    id: 'listening',
    label: 'Listening for joins',
    description: 'Permit join is open.',
    matches: ['starting', 'active'],
  },
  {
    id: 'detected',
    label: 'Device detected',
    description: 'Candidate preparing for interview.',
    matches: ['device_detected', 'device_joined'],
  },
  {
    id: 'interview',
    label: 'Interview in progress',
    description: 'Reading clusters & capabilities.',
    matches: ['interviewing'],
  },
  {
    id: 'finalize',
    label: 'Finalizing provisioning',
    description: 'Saving metadata and enabling automations.',
    matches: ['interview_complete'],
  },
];

function buildPairingPhaseStates(status) {
  const normalized = (status || '').toLowerCase();
  const activeIndex = (() => {
    if (!normalized) return 0;
    const idx = PAIRING_PHASES.findIndex(phase => phase.matches.includes(normalized));
    if (idx >= 0) return idx;
    if (normalized === 'completed') return PAIRING_PHASES.length - 1;
    return 0;
  })();
  return PAIRING_PHASES.map((phase, index) => {
    let state = 'upcoming';
    if (activeIndex > index) {
      state = 'complete';
    } else if (activeIndex === index) {
      state = 'active';
    }
    return { ...phase, state };
  });
}

const defaultForm = {
  protocol: '',
  name: '',
  identifier: '',
  type: '',
  manufacturer: '',
  model: '',
  description: '',
  firmware: '',
  capabilities: '',
  inputs: '',
  icon: 'auto',
};

function getPairingHints(protocol, pairingProfile) {
  const instructions = pairingProfile?.instructions;
  if (Array.isArray(instructions) && instructions.length > 0) {
    return instructions;
  }
  const key = (protocol || '').toLowerCase();
  if (key && Array.isArray(PAIRING_INSTRUCTIONS[key])) {
    return PAIRING_INSTRUCTIONS[key];
  }
  return DEFAULT_PAIRING_INSTRUCTIONS;
}

function getProtocolLabel(protocol, options) {
  if (!protocol) return 'Device';
  const match = options?.find?.(opt => opt?.protocol === protocol);
  if (match?.label) return match.label;
  return protocol.charAt(0).toUpperCase() + protocol.slice(1);
}

function getPairingProfile(protocol, config) {
  const key = (protocol || '').toLowerCase();
  if (!key) return null;
  return config?.[key] || null;
}

function slugify(value) {
  return value
    .toString()
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 60);
}

function parseOptionalJson(raw, label) {
  if (!raw || !raw.trim()) return null;
  try {
    return JSON.parse(raw);
  } catch {
    throw new Error(`${label} must be valid JSON`);
  }
}

function ensureProtocolPrefixedIdentifier(protocol, value) {
  const proto = (protocol || '').toLowerCase();
  const raw = (value || '').trim();
  if (!proto || !raw) return raw;
  const segments = raw.split('/').filter(Boolean);
  const suffix = segments.length ? segments[segments.length - 1] : raw;
  return `${proto}/${suffix}`;
}

function buildIdentifier(protocol, name, manual) {
  const base = (manual && manual.trim()) || slugify(name) || `device-${Date.now().toString(36)}`;
  return ensureProtocolPrefixedIdentifier(protocol, base);
}

export default function AddDeviceModal({
  open,
  onClose,
  onCreate,
  integrations = [],
  pairingSessions = {},
  pairingConfig = {},
  onStartPairing,
  onStopPairing,
}) {
  const [form, setForm] = useState(defaultForm);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState(null);
  const [flowStep, setFlowStep] = useState('form');
  const [activePairing, setActivePairing] = useState(null);
  const [pairingNotice, setPairingNotice] = useState('');
  const [pairingError, setPairingError] = useState(null);
  const [secondsRemaining, setSecondsRemaining] = useState(null);
  const [stopPending, setStopPending] = useState(false);
  const [pairingStartPending, setPairingStartPending] = useState(false);
  const resetFlow = useCallback(() => {
    setFlowStep('form');
    setActivePairing(null);
    setPairingNotice('');
    setPairingError(null);
    setSecondsRemaining(null);
    setStopPending(false);
    setPairingStartPending(false);
    setShowAdvanced(false);
  }, []);
  const modalRef = useRef(null);
  const resumePairingRef = useRef(false);

  const protocolOptions = useMemo(() => {
    const filtered = (integrations || []).filter(item => item && item.protocol && item.status !== 'planned');
    if (filtered.length > 0) {
      return filtered;
    }
    return [
      { protocol: 'zigbee', label: 'Zigbee', status: 'active' },
      { protocol: 'matter', label: 'Matter', status: 'experimental' },
      { protocol: 'thread', label: 'Thread', status: 'experimental' },
    ];
  }, [integrations]);

  const selectedProtocol = form.protocol || protocolOptions[0]?.protocol || '';
  const selectedIntegration = useMemo(
    () => protocolOptions.find(option => option.protocol === selectedProtocol),
    [protocolOptions, selectedProtocol],
  );
  const pairingProfile = useMemo(
    () => getPairingProfile(selectedProtocol, pairingConfig),
    [pairingConfig, selectedProtocol],
  );
  const selectedProtocolLabel = useMemo(
    () => getProtocolLabel(selectedProtocol, protocolOptions),
    [selectedProtocol, protocolOptions],
  );
  const pairingTitleLabel = pairingProfile?.label || selectedProtocolLabel;
  const selectedIcon = form.icon || 'auto';
  const selectedIconMeta = useMemo(() => DEVICE_ICON_CHOICES.find(choice => choice.key === selectedIcon), [selectedIcon]);
  const identifierPreview = useMemo(() => buildIdentifier(selectedProtocol, form.name, form.identifier), [selectedProtocol, form.name, form.identifier]);
  const handleIdentifierChange = useCallback(event => {
    const next = event.target.value.replaceAll('/', '');
    setForm(prev => ({ ...prev, identifier: next }));
  }, []);
  const pairingSupported = useMemo(() => {
    const integrationBlocked = selectedIntegration?.status === 'planned';
    if (typeof pairingProfile?.supported === 'boolean') {
      return pairingProfile.supported && !integrationBlocked;
    }
    return selectedProtocol === 'zigbee' && !integrationBlocked;
  }, [pairingProfile, selectedIntegration, selectedProtocol]);
  const activeSessionForSelected = useMemo(
    () => pairingSessions?.[selectedProtocol],
    [pairingSessions, selectedProtocol],
  );
  const currentStepIndex = useMemo(() => {
    const idx = FLOW_STEPS.findIndex(step => step.id === flowStep);
    return idx >= 0 ? idx : 0;
  }, [flowStep]);
  const progressPercent = useMemo(() => {
    if (FLOW_STEPS.length <= 1) return 100;
    const pct = (currentStepIndex / (FLOW_STEPS.length - 1)) * 100;
    return Math.min(100, Math.max(0, pct));
  }, [currentStepIndex]);

  const activePairingSession = useMemo(() => {
    if (!activePairing || !activePairing.protocol) return null;
    const session = pairingSessions?.[activePairing.protocol];
    if (!session) return null;
    if (activePairing.id && session.id && session.id !== activePairing.id) {
      return null;
    }
    return session;
  }, [activePairing, pairingSessions]);

  const sessionStatus = useMemo(() => {
    if (activePairingSession?.status) return activePairingSession.status;
    if (activePairingSession?.active) return 'active';
    if (activePairing) return activePairing.status || 'starting';
    return 'starting';
  }, [activePairingSession, activePairing]);
  const pairingBlockedReason = useMemo(() => {
    if (pairingSupported) {
      return activeSessionForSelected?.active ? 'Pairing already running for this protocol.' : '';
    }
    if (pairingProfile?.notes) {
      return pairingProfile.notes;
    }
    return 'Guided pairing is not available for this protocol yet.';
  }, [activeSessionForSelected, pairingProfile, pairingSupported]);
  const pairingPhaseStates = useMemo(() => buildPairingPhaseStates(sessionStatus), [sessionStatus]);
  const pairingCtaLabel = pairingProfile?.cta_label || pairingProfile?.ctaLabel || `Start ${pairingTitleLabel} pairing`;

  const handleStopPairing = useCallback(async () => {
    if (!activePairing || typeof onStopPairing !== 'function') {
      resetFlow();
      return;
    }
    try {
      setStopPending(true);
      await onStopPairing(activePairing.protocol);
      resetFlow();
    } catch (err) {
      console.warn('Failed to stop pairing', err);
      setPairingError(err?.message || 'Unable to stop pairing');
    } finally {
      setStopPending(false);
    }
  }, [activePairing, onStopPairing, resetFlow]);

  const handleClose = useCallback(() => {
    if (flowStep === 'pairing' && activePairing && typeof onStopPairing === 'function') {
      onStopPairing(activePairing.protocol).catch(() => {});
    }
    onClose?.();
  }, [flowStep, activePairing, onStopPairing, onClose]);

  const handleSuccessDismiss = useCallback(() => {
    resetFlow();
    setForm(defaultForm);
    setShowAdvanced(false);
    handleClose();
  }, [resetFlow, handleClose]);

  const handleAddAnother = useCallback(() => {
    resetFlow();
    setForm(defaultForm);
    setShowAdvanced(false);
  }, [resetFlow]);

  const buildPairingMetadata = useCallback(() => {
    const normalizedIcon = (form.icon || '').trim().toLowerCase();
    return {
      name: form.name.trim(),
      description: form.description.trim(),
      type: form.type.trim(),
      manufacturer: form.manufacturer.trim(),
      model: form.model.trim(),
      icon: normalizedIcon && normalizedIcon !== 'auto' ? normalizedIcon : '',
    };
  }, [form]);

  const beginGuidedPairing = useCallback(async () => {
    if (!pairingSupported) {
      setPairingError(pairingProfile?.notes || 'Guided pairing is not available for this protocol yet.');
      return;
    }
    if (activeSessionForSelected?.active) {
      setFlowStep('pairing');
      setPairingNotice('Pairing already running. Monitoring progress…');
      return;
    }
    if (typeof onStartPairing !== 'function') {
      setPairingError('Pairing is not available right now.');
      return;
    }
    try {
      setPairingStartPending(true);
      setPairingError(null);
      const timeout = pairingProfile?.default_timeout_sec
        || pairingProfile?.defaultTimeoutSec
        || 60;
      const payload = {
        protocol: selectedProtocol.trim().toLowerCase(),
        timeout,
        metadata: buildPairingMetadata(),
      };
      const response = await onStartPairing(payload);
      const session = response?.data || response;
      if (!session) {
        throw new Error('Pairing session missing in response');
      }
      const startedAt = session.started_at ? new Date(session.started_at) : new Date();
      const expiresAt = session.expires_at ? new Date(session.expires_at) : new Date(startedAt.getTime() + timeout * 1000);
      setActivePairing({
        id: session.id || `${payload.protocol}-${Date.now()}`,
        protocol: payload.protocol,
        startedAt,
        expiresAt,
        status: session.status || 'active',
      });
      setFlowStep('pairing');
      setPairingNotice(pairingProfile?.notes || 'Permit join enabled. Put your device into pairing mode now.');
      setFormError(null);
      resumePairingRef.current = true;
    } catch (err) {
      console.error('Failed to start pairing', err);
      setPairingError(err?.message || 'Unable to start pairing');
    } finally {
      setPairingStartPending(false);
    }
  }, [pairingSupported, activeSessionForSelected, onStartPairing, selectedProtocol, buildPairingMetadata]);

  useEffect(() => {
    if (!open) return undefined;
    const handleKey = event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        handleClose();
      }
    };
    document.addEventListener('keydown', handleKey);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKey);
      document.body.style.overflow = '';
    };
  }, [open, handleClose]);

  useEffect(() => {
    if (!open) return undefined;
    const handleClick = event => {
      if (!modalRef.current) return;
      if (!modalRef.current.contains(event.target)) {
        handleClose();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open, handleClose]);

  useEffect(() => {
    if (!open) {
      resumePairingRef.current = false;
      return;
    }
    if (resumePairingRef.current) {
      return;
    }
    const existing = Object.values(pairingSessions || {}).find(session => session && session.active);
    if (!existing) {
      return;
    }
    const started = existing.startedAt || existing.started_at;
    const expires = existing.expiresAt || existing.expires_at;
    setActivePairing({
      id: existing.id || `${existing.protocol || 'pairing'}-${Date.now()}`,
      protocol: existing.protocol || '',
      startedAt: started instanceof Date ? started : (started ? new Date(started) : new Date()),
      expiresAt: expires instanceof Date ? expires : (expires ? new Date(expires) : null),
    });
    setFlowStep('pairing');
    setPairingNotice('Pairing already running. Monitoring progress…');
    resumePairingRef.current = true;
  }, [open, pairingSessions]);

  useEffect(() => {
    if (open || !activePairing || !onStopPairing) {
      return;
    }
    onStopPairing(activePairing.protocol).catch(() => {});
  }, [open, activePairing, onStopPairing]);

  useEffect(() => {
    if (!open) {
      setForm(defaultForm);
      setShowAdvanced(false);
      setFormError(null);
      resetFlow();
    }
  }, [open, resetFlow]);

  useEffect(() => {
    if (!activePairing) {
      setSecondsRemaining(null);
      return;
    }

    const session = activePairingSession || activePairing;
    const expirationRaw = session?.expiresAt || session?.expires_at;
    if (!expirationRaw) {
      setSecondsRemaining(null);
      return;
    }

    const expiration = expirationRaw instanceof Date ? expirationRaw : new Date(expirationRaw);
    const updateSeconds = () => {
      const diff = expiration.getTime() - Date.now();
      setSecondsRemaining(diff > 0 ? Math.ceil(diff / 1000) : 0);
    };

    updateSeconds();
    const timerId = setInterval(updateSeconds, 1000);
    return () => clearInterval(timerId);
  }, [activePairing, activePairingSession]);

  useEffect(() => {
    if (!activePairingSession) return;
    const { status } = activePairingSession;
    if (status === 'device_joined') {
      setPairingNotice('Device joined the network. Interview starting…');
      setPairingError(null);
      return;
    }
    if (status === 'interviewing') {
      setPairingNotice('Interview in progress. We are reading device capabilities…');
      setPairingError(null);
      return;
    }
    if (status === 'interview_complete') {
      setPairingNotice('Interview complete. Finalizing registration…');
      setPairingError(null);
      return;
    }
    if (status === 'device_detected') {
      setPairingNotice('Device detected. Finalizing…');
      setPairingError(null);
      return;
    }
    if (status === 'completed') {
      setPairingNotice('Device paired successfully.');
      setPairingError(null);
      setFlowStep('success');
      setSecondsRemaining(null);
      setActivePairing(null);
      return;
    }
    if (['timeout', 'failed', 'stopped', 'error'].includes(status)) {
      const errorMessage =
        status === 'timeout'
          ? 'Pairing timed out. Try again after resetting your device.'
          : 'Pairing stopped. You can try again.';
      setPairingError(errorMessage);
      setPairingNotice('');
      setFlowStep('form');
      setActivePairing(null);
      setSecondsRemaining(null);
    }
  }, [activePairingSession]);

  const handleManualCreate = async () => {
    if (!onCreate) return;
    if (!selectedProtocol) {
      setFormError('Select a protocol');
      return;
    }
    try {
      setSubmitting(true);
      setFormError(null);
      const externalId = buildIdentifier(selectedProtocol, form.name, form.identifier);
      if (!externalId) {
        throw new Error('Identifier is required');
      }
      const payload = {
        protocol: selectedProtocol.trim().toLowerCase(),
        external_id: externalId,
      };
      if (form.name.trim()) payload.name = form.name.trim();
      if (form.type.trim()) payload.type = form.type.trim();
      if (form.manufacturer.trim()) payload.manufacturer = form.manufacturer.trim();
      if (form.model.trim()) payload.model = form.model.trim();
      if (form.description.trim()) payload.description = form.description.trim();
      if (form.firmware.trim()) payload.firmware = form.firmware.trim();
      const capabilities = parseOptionalJson(form.capabilities, 'Capabilities');
      const inputs = parseOptionalJson(form.inputs, 'Inputs');
      if (capabilities) payload.capabilities = capabilities;
      if (inputs) payload.inputs = inputs;
      const normalizedIcon = (form.icon || '').trim().toLowerCase();
      if (normalizedIcon && normalizedIcon !== 'auto') {
        payload.icon = normalizedIcon;
      }
      await onCreate(payload);
      setSubmitting(false);
      setForm(defaultForm);
      handleClose();
    } catch (err) {
      setSubmitting(false);
      setFormError(err?.message || 'Unable to create device');
    }
  };

  const handleBackdropMouseDown = event => {
    if (event.target === event.currentTarget) {
      handleClose();
    }
  };

  const renderPairingStep = () => {
    const countdownLabel = secondsRemaining ?? '—';
    const statusLabel = sessionStatus.replace(/_/g, ' ');
    const pairingProtocol = activePairing?.protocol || selectedProtocol;
    const pairingProfileForView = getPairingProfile(pairingProtocol, pairingConfig) || pairingProfile;
    const pairingHints = getPairingHints(pairingProtocol, pairingProfileForView);
    const pairingMeta = activePairingSession?.metadata;
    const pairingDeviceId = activePairingSession?.deviceId || activePairingSession?.device_id;
    const fallbackPairingNotice = pairingProfileForView?.notes || 'Permit join active — reset the device and keep it near the coordinator.';
    return (
      <div className="auth-modal-content-inner add-device-step add-device-pairing-step">
        <div className="add-device-pairing-panel">
          <div className="add-device-pairing-progress">
            <div className="add-device-pairing-stages">
              {pairingPhaseStates.map((phase, index) => (
                <div key={phase.id} className={`add-device-pairing-stage-card ${phase.state}`}>
                  <span className="add-device-pairing-stage-index">{index + 1}</span>
                  <div className="add-device-pairing-stage-body">
                    <span className="add-device-pairing-stage-label">{phase.label}</span>
                    <p>{phase.description}</p>
                  </div>
                </div>
              ))}
            </div>
            <div className="add-device-pairing-timer">
              <div>
                <span className="add-device-pairing-label">Time left</span>
                <div className="add-device-countdown-number">{countdownLabel}</div>
              </div>
              <div className="add-device-pairing-status-pill">{statusLabel}</div>
            </div>
          </div>
          <p className="add-device-pairing-note">{pairingNotice || fallbackPairingNotice}</p>
          <div className="add-device-pairing-inline-tips">
            {pairingHints.map(instruction => (
              <span key={instruction}>{instruction}</span>
            ))}
          </div>
          {pairingMeta ? (
            <div className="add-device-pairing-meta">
              <span className="add-device-pairing-label">Metadata applied after join</span>
              <div className="add-device-pairing-meta-grid">
                {pairingMeta.name ? <span><strong>Name:</strong> {pairingMeta.name}</span> : null}
                {pairingMeta.type ? <span><strong>Type:</strong> {pairingMeta.type}</span> : null}
                {pairingMeta.manufacturer ? <span><strong>Mfr:</strong> {pairingMeta.manufacturer}</span> : null}
                {pairingMeta.model ? <span><strong>Model:</strong> {pairingMeta.model}</span> : null}
                {pairingMeta.icon ? <span><strong>Icon:</strong> {pairingMeta.icon}</span> : null}
                {pairingDeviceId ? <span><strong>Device ID:</strong> {pairingDeviceId}</span> : null}
              </div>
            </div>
          ) : null}
          {pairingError ? <div className="auth-modal-error add-device-error">{pairingError}</div> : null}
          <div className="add-device-pairing-actions">
            <button type="button" className="auth-modal-btn" onClick={handleStopPairing} disabled={stopPending}>
              {stopPending ? 'Stopping…' : 'Stop pairing'}
            </button>
          </div>
        </div>
      </div>
    );
  };

  const renderSuccessStep = () => (
    <div className="auth-modal-content-inner add-device-step add-device-success-step">
      <FontAwesomeIcon icon={faCircleCheck} className="add-device-success-icon" />
      <h3>Device connected</h3>
      <p>Your device is paired and syncing events automatically.</p>
      <div className="add-device-success-actions">
        <button type="button" className="auth-modal-btn" onClick={handleSuccessDismiss}>
          Done
        </button>
        <button type="button" className="add-device-secondary-btn" onClick={handleAddAnother}>
          Add another
        </button>
      </div>
    </div>
  );

  const headerTitleMap = {
    form: 'Add Device',
    pairing: `Guided ${pairingTitleLabel} Pairing`,
    success: 'Device Connected',
  };
  const headerTitle = headerTitleMap[flowStep] || headerTitleMap.form;

  if (!open) {
    return null;
  }

  const modal = (
    <div
      className={`auth-modal-backdrop add-device-modal-backdrop${open ? ' open' : ''}`}
      onMouseDown={handleBackdropMouseDown}
    >
      <div
        className={`auth-modal-glass add-device-modal-glass${open ? ' open' : ''}`}
        ref={modalRef}
      >
        <button type="button" className="auth-modal-close" onClick={handleClose} aria-label="Close add device dialog">
          ×
        </button>
        <div className="auth-modal-content add-device-shell">
          <div className="add-device-toolbar">
            <div className="add-device-toolbar-title">
              <div className="add-device-toolbar-heading">
                <h2>{headerTitle}</h2>
              </div>
            </div>
            <div className="add-device-progress-bar add-device-toolbar-progress">
              <span style={{ width: `${progressPercent}%` }} />
            </div>
          </div>
          <div className="add-device-step-tabs">
            {FLOW_STEPS.map((step, index) => {
              const status = index < currentStepIndex ? 'complete' : index === currentStepIndex ? 'active' : 'upcoming';
              return (
                <div key={step.id} className={`add-device-step-tab ${status}`}>
                  <span className="add-device-step-index">0{index + 1}</span>
                  <span className="add-device-step-label">{step.label}</span>
                </div>
              );
            })}
          </div>
          <div className="auth-modal-content-outer add-device-scroll-region">
            {flowStep === 'form' ? (
              <div className="auth-modal-content-inner add-device-form-shell">
                <form className="auth-modal-form add-device-form" onSubmit={event => event.preventDefault()} noValidate>
                  <div className="add-device-body-grid">
                    <div className="add-device-panel add-device-panel-primary">
                      <div className="add-device-card add-device-card-emphasis">
                        <div className="add-device-card-head add-device-card-head-center">
                          <div>
                            <h4>Device details</h4>
                            <p>Share the essentials now; you can polish the details anytime.</p>
                          </div>
                        </div>
                        <div className="add-device-grid add-device-basics-grid">
                          <div className={`auth-modal-field add-device-field add-device-field-select add-device-grid-span-2${selectedProtocol ? ' filled' : ''}`}>
                            <select
                              id="add-device-protocol"
                              className="auth-modal-input add-device-select"
                              value={selectedProtocol}
                              onChange={event => setForm(prev => ({ ...prev, protocol: event.target.value }))}
                              required
                            >
                              {protocolOptions.map(option => (
                                <option key={option.protocol} value={option.protocol}>
                                  {option.label || option.protocol}
                                  {option.status ? ` (${option.status})` : ''}
                                </option>
                              ))}
                            </select>
                            <label className="auth-modal-label" htmlFor="add-device-protocol">Protocol</label>
                          </div>
                          <div className="auth-modal-field add-device-field">
                            <input
                              id="add-device-name"
                              className="auth-modal-input"
                              type="text"
                              placeholder=" "
                              value={form.name}
                              onChange={event => setForm(prev => ({ ...prev, name: event.target.value }))}
                            />
                            <label className="auth-modal-label" htmlFor="add-device-name">Name (optional)</label>
                          </div>
                          <div className="auth-modal-field add-device-field">
                            <input
                              id="add-device-identifier"
                              className="auth-modal-input"
                              type="text"
                              placeholder=" "
                              value={form.identifier}
                              onChange={handleIdentifierChange}
                            />
                            <label className="auth-modal-label" htmlFor="add-device-identifier">Identifier (optional)</label>
                            <small className="add-device-modal-hint">Preview: {identifierPreview}</small>
                          </div>
                        </div>
                        <div className="add-device-icon-field">
                          <div className="add-device-icon-label">Icon</div>
                          <div className="device-icon-picker-grid add-device-icon-grid">
                            {DEVICE_ICON_CHOICES.map(choice => (
                              <button
                                key={choice.key}
                                type="button"
                                className={`device-icon-choice${selectedIcon === choice.key ? ' active' : ''}`}
                                onClick={() => setForm(prev => ({ ...prev, icon: choice.key }))}
                              >
                                <FontAwesomeIcon icon={choice.icon} />
                                <span>{choice.label}</span>
                              </button>
                            ))}
                          </div>
                        </div>
                      </div>

                      <div className="add-device-card add-device-advanced-card">
                        <button
                          type="button"
                          className={`add-device-advanced-toggle${showAdvanced ? ' open' : ''}`}
                          onClick={() => setShowAdvanced(prev => !prev)}
                          aria-expanded={showAdvanced}
                        >
                          <div>
                            <span>Advanced setup & metadata</span>
                            <p>Manufacturer, firmware, payloads, and manual registration.</p>
                          </div>
                          <FontAwesomeIcon icon={faChevronDown} />
                        </button>
                        <div className={`add-device-advanced${showAdvanced ? ' open' : ''}`}>
                          <div className="add-device-grid add-device-advanced-grid">
                            <div className="auth-modal-field add-device-field">
                              <input
                                id="add-device-type"
                                className="auth-modal-input"
                                type="text"
                                placeholder=" "
                                value={form.type}
                                onChange={event => setForm(prev => ({ ...prev, type: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-type">Type</label>
                            </div>
                            <div className="auth-modal-field add-device-field">
                              <input
                                id="add-device-manufacturer"
                                className="auth-modal-input"
                                type="text"
                                placeholder=" "
                                value={form.manufacturer}
                                onChange={event => setForm(prev => ({ ...prev, manufacturer: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-manufacturer">Manufacturer</label>
                            </div>
                            <div className="auth-modal-field add-device-field">
                              <input
                                id="add-device-model"
                                className="auth-modal-input"
                                type="text"
                                placeholder=" "
                                value={form.model}
                                onChange={event => setForm(prev => ({ ...prev, model: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-model">Model</label>
                            </div>
                            <div className="auth-modal-field add-device-field add-device-grid-span-2 add-device-description-field">
                              <textarea
                                id="add-device-description"
                                className="auth-modal-input add-device-textarea"
                                rows={2}
                                placeholder=" "
                                value={form.description}
                                onChange={event => setForm(prev => ({ ...prev, description: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-description">Description</label>
                            </div>
                            <div className="auth-modal-field add-device-field">
                              <input
                                id="add-device-firmware"
                                className="auth-modal-input"
                                type="text"
                                placeholder=" "
                                value={form.firmware}
                                onChange={event => setForm(prev => ({ ...prev, firmware: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-firmware">Firmware</label>
                            </div>
                            <div className="auth-modal-field add-device-field add-device-grid-span-2">
                              <textarea
                                id="add-device-capabilities"
                                className="auth-modal-input add-device-textarea"
                                rows={3}
                                placeholder=" "
                                value={form.capabilities}
                                onChange={event => setForm(prev => ({ ...prev, capabilities: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-capabilities">Capabilities JSON</label>
                              <small className="add-device-modal-hint">Example: {CAPABILITIES_EXAMPLE}</small>
                            </div>
                            <div className="auth-modal-field add-device-field add-device-grid-span-2">
                              <textarea
                                id="add-device-inputs"
                                className="auth-modal-input add-device-textarea"
                                rows={3}
                                placeholder=" "
                                value={form.inputs}
                                onChange={event => setForm(prev => ({ ...prev, inputs: event.target.value }))}
                              />
                              <label className="auth-modal-label" htmlFor="add-device-inputs">Inputs JSON</label>
                              <small className="add-device-modal-hint">Example: {INPUTS_EXAMPLE}</small>
                            </div>
                          </div>
                          <div className="add-device-advanced-actions">
                            {formError ? <div className="auth-modal-error add-device-error">{formError}</div> : null}
                            <button
                              type="button"
                              className="add-device-secondary-btn add-device-manual-btn"
                              onClick={handleManualCreate}
                              disabled={submitting}
                            >
                              {submitting ? 'Saving…' : 'Register manually'}
                            </button>
                            <small className="add-device-modal-hint">Use this for MQTT, HTTP, or other advanced integrations.</small>
                          </div>
                        </div>
                      </div>

                      <div className="add-device-card add-device-guided-card">
                        <div className="add-device-card-head add-device-card-head-center">
                          <div>
                            <h4>{pairingSupported ? `Guided ${pairingTitleLabel} pairing` : 'Manual onboarding'}</h4>
                              <p>{pairingSupported
                                ? (pairingProfile?.notes || 'Kick off permit-join right when you reset the device.')
                                : 'Use advanced setup or manual registration for this protocol.'}
                              </p>
                          </div>
                        </div>
                        {pairingError ? <div className="auth-modal-error add-device-error">{pairingError}</div> : null}
                        <div className="add-device-guided-actions">
                          {pairingSupported ? (
                            <button
                              type="button"
                              className="auth-modal-btn add-device-guided-action"
                              onClick={beginGuidedPairing}
                              disabled={pairingStartPending || activeSessionForSelected?.active}
                            >
                              {activeSessionForSelected?.active
                                ? 'Pairing in progress'
                                : pairingStartPending
                                  ? 'Starting…'
                                  : pairingCtaLabel}
                            </button>
                          ) : (
                            <button
                              type="button"
                              className="add-device-secondary-btn add-device-guided-action"
                              onClick={() => setShowAdvanced(true)}
                            >
                              Open advanced setup
                            </button>
                          )}
                          <small className="add-device-modal-hint">
                            {pairingSupported
                              ? (pairingBlockedReason || 'Need manual control? Expand Advanced setup above.')
                              : (pairingBlockedReason || 'Guided pairing is not available for this protocol yet.')}
                          </small>
                        </div>
                      </div>
                    </div>

                  </div>
                </form>
              </div>
            ) : flowStep === 'pairing' ? (
              renderPairingStep()
            ) : (
              renderSuccessStep()
            )}
          </div>
        </div>
      </div>
    </div>
  );

  if (typeof document === 'undefined') {
    return modal;
  }

  return createPortal(modal, getModalRoot());
}
