import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChevronDown, faCircleCheck } from '@fortawesome/free-solid-svg-icons';
import { DEVICE_ICON_CHOICES } from './deviceIconChoices';
import BaseModal from '../common/BaseModal/BaseModal';
import PairingFlowRenderer from './pairing/PairingFlowRenderer';
import PairingProgressPanel from './pairing/PairingProgressPanel';
import {
  buildPairingStartPayload,
  deriveRecoveryPreset,
  getBlockedReason,
  getPairingProfile,
  getProtocolOptions,
  isPairingSupported,
} from './pairing/pairingSchema';
import './AddDeviceModal.css';

const FLOW_STEPS = [
  { id: 'form', label: 'Details' },
  { id: 'pairing', label: 'Pairing' },
  { id: 'success', label: 'Complete' },
];

const defaultForm = {
  protocol: '',
  name: '',
  type: '',
  manufacturer: '',
  model: '',
  description: '',
  icon: 'auto',
};

function getProtocolLabel(protocol, options) {
  if (!protocol) return 'Device';
  const match = options?.find?.(opt => opt?.protocol === protocol);
  if (match?.label) return match.label;
  return protocol.charAt(0).toUpperCase() + protocol.slice(1);
}

function normalizeErrorCode(value) {
  return `${value || ''}`.trim().toUpperCase();
}

function buildTerminalRecoveryCopy(context, protocolLabel) {
  const status = `${context?.status || ''}`.trim().toLowerCase();
  const requiredInputs = Array.isArray(context?.requiredInputs)
    ? context.requiredInputs.filter(Boolean)
    : [];
  const fallbackLabel = protocolLabel || 'device';

  const response = {
    summary: context?.message || `Pairing did not complete for this ${fallbackLabel.toLowerCase()}.`,
    recommendations: [
      'Retry with previous inputs to continue quickly.',
      'Change mode if the current path is unstable for this device.',
    ],
    primaryAction: 'retry',
    retryLabel: 'Retry with previous inputs',
    modeLabel: 'Change mode',
    detailsLabel: 'Return to details',
  };

  if (status === 'timeout') {
    response.summary = `Pairing timed out before ${fallbackLabel} commissioning completed.`;
    response.recommendations = [
      'Reset or wake the device, then retry immediately.',
      'Keep the device near the coordinator during the full flow.',
    ];
    response.retryLabel = 'Retry pairing now';
  }

  if (status === 'stopped') {
    response.summary = `Pairing was stopped before ${fallbackLabel} setup finished.`;
    response.recommendations = [
      'Retry to resume with the same values.',
      'Use a different mode if the device behavior changed.',
    ];
    response.retryLabel = 'Start pairing again';
  }

  if (requiredInputs.length > 0) {
    response.primaryAction = 'mode';
    const normalizedError = normalizeErrorCode(context?.errorCode);
    if (normalizedError) {
      response.summary = `Pairing requires additional input (${normalizedError}).`;
    } else {
      response.summary = 'Pairing requires additional input before continuing.';
    }
    response.recommendations = [
      `Required inputs: ${requiredInputs.join(', ')}`,
      ...response.recommendations,
    ];
    response.retryLabel = 'Retry after updating inputs';
    response.modeLabel = 'Edit required inputs';
  }

  return response;
}

export default function AddDeviceModal({
  open,
  onClose,
  integrations = [],
  pairingSessions = {},
  pairingConfig = {},
  onStartPairing,
  onStopPairing,
}) {
  const [form, setForm] = useState(defaultForm);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [flowStep, setFlowStep] = useState('form');
  const [activePairing, setActivePairing] = useState(null);
  const [pairingNotice, setPairingNotice] = useState('');
  const [pairingError, setPairingError] = useState(null);
  const [secondsRemaining, setSecondsRemaining] = useState(null);
  const [stopPending, setStopPending] = useState(false);
  const [pairingStartPending, setPairingStartPending] = useState(false);
  const [pairingSetupOverride, setPairingSetupOverride] = useState(false);
  const [pairingRecoveryDraft, setPairingRecoveryDraft] = useState(null);
  const [pairingTerminalContext, setPairingTerminalContext] = useState(null);

  const modalRef = useRef(null);
  const resumePairingRef = useRef(false);

  const resetFlow = useCallback(() => {
    setFlowStep('form');
    setActivePairing(null);
    setPairingNotice('');
    setPairingError(null);
    setSecondsRemaining(null);
    setStopPending(false);
    setPairingStartPending(false);
    setShowAdvanced(false);
    setPairingSetupOverride(false);
    setPairingRecoveryDraft(null);
    setPairingTerminalContext(null);
  }, []);

  const protocolOptions = useMemo(
    () => getProtocolOptions(integrations, pairingConfig),
    [integrations, pairingConfig],
  );

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

  const pairingSupported = useMemo(
    () => isPairingSupported(pairingProfile, selectedIntegration?.status || ''),
    [pairingProfile, selectedIntegration],
  );

  const activeSessionForSelected = useMemo(
    () => pairingSessions?.[selectedProtocol],
    [pairingSessions, selectedProtocol],
  );

  const pairingBlockedReason = useMemo(
    () => getBlockedReason(pairingSupported, activeSessionForSelected, pairingProfile),
    [pairingSupported, activeSessionForSelected, pairingProfile],
  );

  const activePairingSession = useMemo(() => {
    if (!activePairing || !activePairing.protocol) return null;
    return pairingSessions?.[activePairing.protocol] || null;
  }, [activePairing, pairingSessions]);

  const sessionStatus = useMemo(() => {
    if (activePairingSession?.status) return activePairingSession.status;
    if (activePairingSession?.active) return 'active';
    if (activePairing) return activePairing.status || 'starting';
    return 'starting';
  }, [activePairingSession, activePairing]);

  // Stage is a finer-grained progress signal from the adapter.
  // Use it in addition to status so the step timeline can advance properly.
  const sessionStage = activePairingSession?.stage || '';

  const pairingCtaLabel = pairingProfile?.cta_label || pairingProfile?.ctaLabel || `Start ${pairingTitleLabel} pairing`;
  const canEnterPairingFlow = Boolean(selectedProtocol) && pairingSupported;
  const terminalRecoveryCopy = useMemo(
    () => buildTerminalRecoveryCopy(pairingTerminalContext, pairingTitleLabel),
    [pairingTerminalContext, pairingTitleLabel],
  );
  const terminalActionOrder = useMemo(() => {
    const primary = `${terminalRecoveryCopy?.primaryAction || 'retry'}`.trim().toLowerCase();
    if (primary === 'mode') return ['mode', 'retry', 'details'];
    if (primary === 'details') return ['details', 'retry', 'mode'];
    return ['retry', 'mode', 'details'];
  }, [terminalRecoveryCopy]);

  const currentStepIndex = useMemo(() => {
    const idx = FLOW_STEPS.findIndex(step => step.id === flowStep);
    return idx >= 0 ? idx : 0;
  }, [flowStep]);

  const progressPercent = useMemo(() => {
    if (FLOW_STEPS.length <= 1) return 100;
    const pct = (currentStepIndex / (FLOW_STEPS.length - 1)) * 100;
    return Math.min(100, Math.max(0, pct));
  }, [currentStepIndex]);

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

  const beginGuidedPairing = useCallback(async payload => {
    if (!pairingSupported) {
      setPairingError(pairingBlockedReason || 'Guided pairing is not available for this protocol yet.');
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
      const metadata = buildPairingMetadata();
      const response = await onStartPairing({
        ...payload,
        protocol: selectedProtocol.trim().toLowerCase(),
        metadata,
      });
      const session = response?.data || response;
      if (!session) {
        throw new Error('Pairing session missing in response');
      }
      const startedAt = session.started_at ? new Date(session.started_at) : new Date();
      const expiresAt = session.expires_at ? new Date(session.expires_at) : new Date(startedAt.getTime() + Number(payload?.timeout || 180) * 1000);
      setActivePairing({
        id: session.id || `${selectedProtocol}-${Date.now()}`,
        protocol: selectedProtocol,
        startedAt,
        expiresAt,
        status: session.status || 'active',
      });
      setFlowStep('pairing');
      setPairingSetupOverride(false);
      setPairingRecoveryDraft(null);
      setPairingTerminalContext(null);
      setPairingNotice(pairingProfile?.notes || 'Permit join enabled. Put your device into pairing mode now.');
      resumePairingRef.current = true;
    } catch (error) {
      setPairingError(error?.message || 'Unable to start pairing');
    } finally {
      setPairingStartPending(false);
    }
  }, [
    activeSessionForSelected,
    buildPairingMetadata,
    onStartPairing,
    pairingBlockedReason,
    pairingProfile,
    pairingSupported,
    selectedProtocol,
  ]);

  const handleStopPairing = useCallback(async () => {
    const protocolToStop = activePairing?.protocol
      || activePairingSession?.protocol
      || (activeSessionForSelected?.active ? activeSessionForSelected.protocol : '')
      || '';
    if (!protocolToStop || typeof onStopPairing !== 'function') {
      resetFlow();
      return;
    }
    try {
      setStopPending(true);
      await onStopPairing(protocolToStop);
      resetFlow();
    } catch (error) {
      setPairingError(error?.message || 'Unable to stop pairing');
    } finally {
      setStopPending(false);
    }
  }, [activePairing, activePairingSession, activeSessionForSelected, onStopPairing, resetFlow]);

  const handleClose = useCallback(() => {
    if (typeof onStopPairing === 'function') {
      const protocolToStop = (activePairing || activePairingSession?.active || activeSessionForSelected?.active)
        ? ((activePairing?.protocol || activePairingSession?.protocol || '')
          || (activeSessionForSelected?.active ? activeSessionForSelected.protocol : '')
          || '')
        : '';
      if (protocolToStop) {
        onStopPairing(protocolToStop).catch(() => {});
      }
    }
    onClose?.();
  }, [activePairing, activePairingSession, activeSessionForSelected, onStopPairing, onClose]);

  const handleSuccessDismiss = useCallback(() => {
    resetFlow();
    setForm(defaultForm);
    handleClose();
  }, [resetFlow, handleClose]);

  const handleAddAnother = useCallback(() => {
    resetFlow();
    setForm(defaultForm);
  }, [resetFlow]);

  const handleEnterPairingFlow = useCallback(() => {
    setPairingError(null);
    setPairingNotice('');
    setPairingSetupOverride(true);
    setPairingRecoveryDraft(null);
    setPairingTerminalContext(null);
    setFlowStep('pairing');
  }, []);

  const handleRetryPairing = useCallback(async () => {
    if (!pairingProfile || !selectedProtocol) {
      setPairingError('Pairing profile is unavailable for retry.');
      return;
    }

    const mode = (pairingRecoveryDraft?.mode || '').trim().toLowerCase()
      || (((pairingProfile?.flow?.entry_modes || [])[0] || '').trim().toLowerCase())
      || 'default';
    const payload = buildPairingStartPayload({
      protocol: selectedProtocol,
      pairingProfile,
      selectedMode: mode,
      fieldValues: pairingRecoveryDraft?.values || {},
    });
    await beginGuidedPairing(payload);
  }, [beginGuidedPairing, pairingProfile, pairingRecoveryDraft, selectedProtocol]);

  const handleChangeMode = useCallback(() => {
    const recoveryPreset = deriveRecoveryPreset({
      pairingProfile,
      currentMode: pairingRecoveryDraft?.mode || '',
      currentValues: pairingRecoveryDraft?.values || {},
      requiredFieldIds: pairingRecoveryDraft?.requiredFieldIds || [],
      terminalStatus: pairingTerminalContext?.status || '',
      errorCode: pairingTerminalContext?.errorCode || '',
      preferAlternateMode: true,
    });
    setPairingError(null);
    setPairingNotice('Choose a different mode and continue pairing.');
    setPairingSetupOverride(true);
    setPairingRecoveryDraft(recoveryPreset);
    setPairingTerminalContext(null);
    setFlowStep('pairing');
  }, [pairingProfile, pairingRecoveryDraft, pairingTerminalContext]);

  const handleReturnToDetails = useCallback(() => {
    setPairingSetupOverride(false);
    setPairingRecoveryDraft(null);
    setPairingTerminalContext(null);
    setFlowStep('form');
  }, []);

  const handleResolveNeedsInput = useCallback(async () => {
    const protocol = activePairing?.protocol || activePairingSession?.protocol || selectedProtocol;
    if (!protocol) {
      setPairingError('Unable to resolve required inputs for pairing session.');
      return;
    }

    const requiredFieldIds = Array.isArray(activePairingSession?.requiredInputs)
      ? activePairingSession.requiredInputs
      : [];
    const recoveryValues = activePairingSession?.progress?.inputs && typeof activePairingSession.progress.inputs === 'object'
      ? { ...activePairingSession.progress.inputs }
      : (activePairingSession?.inputs && typeof activePairingSession.inputs === 'object' ? { ...activePairingSession.inputs } : {});

    try {
      setStopPending(true);
      if (typeof onStopPairing === 'function') {
        await onStopPairing(protocol);
      }
      setPairingSetupOverride(true);
      setPairingRecoveryDraft({
        mode: activePairingSession?.mode || '',
        values: recoveryValues,
        requiredFieldIds,
      });
      setPairingTerminalContext(null);
      setPairingError(null);
      setPairingNotice('Provide the missing fields and continue pairing.');
      setActivePairing(null);
    } catch (error) {
      setPairingError(error?.message || 'Unable to switch to required input flow.');
    } finally {
      setStopPending(false);
    }
  }, [activePairing, activePairingSession, onStopPairing, selectedProtocol]);

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
    if (!open) {
      setForm(defaultForm);
      resetFlow();
    }
  }, [open, resetFlow]);

  useEffect(() => {
    if (!activePairing) {
      setSecondsRemaining(null);
      return;
    }

    const session = activePairingSession || activePairing;
    const sessionStatusForTimer = (activePairingSession?.status || '').toLowerCase();
    if (['interviewing', 'interview_complete', 'completed'].includes(sessionStatusForTimer)) {
      setSecondsRemaining(null);
      return;
    }
    // Prefer the expiresAt we stored from the API response when the session
    // started — device-hub pairing-progress republishes don't carry expires_at.
    const expirationRaw = activePairing?.expiresAt
      || activePairing?.expires_at
      || session?.expiresAt
      || session?.expires_at;
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
    if (status === 'needs_input') {
      setPairingNotice(activePairingSession?.message || 'Additional pairing input is required.');
      setPairingError(null);
      setPairingTerminalContext(null);
      return;
    }
    if (status === 'completed') {
      setPairingNotice('Device paired successfully.');
      setPairingError(null);
      setFlowStep('success');
      setSecondsRemaining(null);
      setActivePairing(null);
      setPairingSetupOverride(false);
      setPairingRecoveryDraft(null);
      setPairingTerminalContext(null);
      return;
    }
    if (['timeout', 'failed', 'stopped', 'error'].includes(status)) {
      const errorMessage =
        status === 'timeout'
          ? 'Pairing timed out. Try again after resetting your device.'
          : (activePairingSession?.message || 'Pairing stopped. You can try again.');
      setPairingError(errorMessage);
      setPairingNotice('');
      setFlowStep('pairing');
      setActivePairing(null);
      setPairingSetupOverride(true);
      const terminalInputs = activePairingSession?.progress?.inputs && typeof activePairingSession.progress.inputs === 'object'
        ? { ...activePairingSession.progress.inputs }
        : (activePairingSession?.inputs && typeof activePairingSession.inputs === 'object'
          ? { ...activePairingSession.inputs }
          : {});
      const terminalErrorCode = activePairingSession?.errorCode
        || activePairingSession?.error_code
        || activePairingSession?.progress?.error_code
        || '';
      const recoveryPreset = deriveRecoveryPreset({
        pairingProfile,
        currentMode: activePairingSession?.mode || '',
        currentValues: terminalInputs,
        requiredFieldIds: Array.isArray(activePairingSession?.requiredInputs)
          ? activePairingSession.requiredInputs
          : [],
        terminalStatus: status,
        errorCode: terminalErrorCode,
      });
      setPairingRecoveryDraft(recoveryPreset);
      setPairingTerminalContext({
        status,
        message: activePairingSession?.message || '',
        errorCode: terminalErrorCode,
        requiredInputs: Array.isArray(activePairingSession?.requiredInputs)
          ? activePairingSession.requiredInputs
          : [],
      });
      setSecondsRemaining(null);
    }
  }, [activePairingSession, pairingProfile]);

  const handleBackdropMouseDown = event => {
    if (event.target === event.currentTarget) {
      handleClose();
    }
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
    <BaseModal
      open={open}
      onClose={handleClose}
      dialogClassName="add-device-modal-glass"
      backdropClassName="add-device-modal-backdrop"
      onBackdropMouseDown={handleBackdropMouseDown}
      closeAriaLabel="Close add device dialog"
    >
      <div className="auth-modal-content add-device-shell" ref={modalRef}>
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
                          <p>Protocol-independent parameters used as pairing metadata.</p>
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
                            id="add-device-type"
                            className="auth-modal-input"
                            type="text"
                            placeholder=" "
                            value={form.type}
                            onChange={event => setForm(prev => ({ ...prev, type: event.target.value }))}
                          />
                          <label className="auth-modal-label" htmlFor="add-device-type">Type</label>
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
                          <span>Additional metadata</span>
                          <p>Attach richer protocol-independent details to pairing.</p>
                        </div>
                        <FontAwesomeIcon icon={faChevronDown} />
                      </button>

                      <div className={`add-device-advanced${showAdvanced ? ' open' : ''}`}>
                        <div className="add-device-grid add-device-advanced-grid">
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
                        </div>
                      </div>
                    </div>

                    <div className="add-device-card add-device-guided-card">
                      <div className="add-device-card-head add-device-card-head-center">
                        <div>
                          <h4>{pairingSupported ? 'Ready for pairing flow' : 'Guided pairing unavailable'}</h4>
                          <p>
                            {pairingSupported
                              ? 'Continue to Step 02 for adapter-guided pairing instructions and runtime progress.'
                              : (pairingBlockedReason || 'This protocol does not currently publish a guided pairing schema.')}
                          </p>
                        </div>
                      </div>

                      <div className="add-device-guided-actions">
                        <button
                          type="button"
                          className="auth-modal-btn add-device-guided-action"
                          onClick={handleEnterPairingFlow}
                          disabled={!canEnterPairingFlow}
                        >
                          Continue to pairing flow
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </form>
            </div>
          ) : flowStep === 'pairing' ? (
            (!pairingSetupOverride && (activePairingSession || activePairing)) ? (
              <PairingProgressPanel
                activePairing={activePairing}
                activePairingSession={activePairingSession}
                selectedProtocol={selectedProtocol}
                pairingConfig={pairingConfig}
                fallbackPairingProfile={pairingProfile}
                sessionStatus={sessionStatus}
                sessionStage={sessionStage}
                pairingNotice={pairingNotice}
                pairingError={pairingError}
                secondsRemaining={secondsRemaining}
                onStop={handleStopPairing}
                stopPending={stopPending}
                onResolveNeedsInput={handleResolveNeedsInput}
              />
            ) : (
              <div className="auth-modal-content-inner add-device-step add-device-pairing-step">
                <div className="add-device-pairing-panel">
                  <div className="add-device-card-head add-device-card-head-center">
                    <div>
                      <h4>{pairingSupported ? `Guided ${pairingTitleLabel} pairing` : 'Guided pairing unavailable'}</h4>
                      <p>
                        {pairingSupported
                          ? (pairingProfile?.notes || 'Follow the guided steps below to start pairing.')
                          : (pairingBlockedReason || 'This protocol does not currently publish a guided pairing schema.')}
                      </p>
                    </div>
                  </div>

                  {pairingError ? <div className="auth-modal-error add-device-error">{pairingError}</div> : null}

                  {pairingSupported ? (
                    <PairingFlowRenderer
                      protocol={selectedProtocol}
                      pairingProfile={pairingProfile}
                      disabled={Boolean(activeSessionForSelected?.active)}
                      busy={pairingStartPending}
                      ctaLabel={activeSessionForSelected?.active ? 'Pairing in progress' : pairingCtaLabel}
                      blockedReason={pairingBlockedReason}
                      onError={setPairingError}
                      onStart={beginGuidedPairing}
                      initialMode={pairingRecoveryDraft?.mode || ''}
                      initialValues={pairingRecoveryDraft?.values || null}
                      requiredFieldIds={pairingRecoveryDraft?.requiredFieldIds || []}
                    />
                  ) : (
                    <div className="add-device-guided-actions">
                      <button type="button" className="auth-modal-btn add-device-guided-action" disabled>
                        Guided pairing unavailable
                      </button>
                    </div>
                  )}

                  {pairingError && pairingRecoveryDraft ? (
                    <div className="add-device-pairing-terminal-actions">
                      <div className="add-device-pairing-terminal-guidance">
                        <p>{terminalRecoveryCopy.summary}</p>
                        {terminalRecoveryCopy.recommendations.length > 0 ? (
                          <ul>
                            {terminalRecoveryCopy.recommendations.map(item => (
                              <li key={item}>{item}</li>
                            ))}
                          </ul>
                        ) : null}
                      </div>
                      {terminalActionOrder.map(actionId => {
                        if (actionId === 'retry') {
                          return (
                            <button
                              key="retry"
                              type="button"
                              className={terminalRecoveryCopy.primaryAction === 'retry' ? 'auth-modal-btn' : 'add-device-secondary-btn'}
                              onClick={handleRetryPairing}
                              disabled={pairingStartPending}
                            >
                              {pairingStartPending ? 'Retrying…' : terminalRecoveryCopy.retryLabel}
                            </button>
                          );
                        }
                        if (actionId === 'mode') {
                          return (
                            <button
                              key="mode"
                              type="button"
                              className={terminalRecoveryCopy.primaryAction === 'mode' ? 'auth-modal-btn' : 'add-device-secondary-btn'}
                              onClick={handleChangeMode}
                              disabled={pairingStartPending}
                            >
                              {terminalRecoveryCopy.modeLabel}
                            </button>
                          );
                        }
                        return (
                          <button
                            key="details"
                            type="button"
                            className={terminalRecoveryCopy.primaryAction === 'details' ? 'auth-modal-btn' : 'add-device-secondary-btn'}
                            onClick={handleReturnToDetails}
                            disabled={pairingStartPending}
                          >
                            {terminalRecoveryCopy.detailsLabel}
                          </button>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              </div>
            )
          ) : (
            renderSuccessStep()
          )}
        </div>
      </div>
    </BaseModal>
  );

  if (typeof document === 'undefined') {
    return modal;
  }

  return modal;
}
