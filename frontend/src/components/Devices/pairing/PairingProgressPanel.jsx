import React, { useMemo } from 'react';
import {
  buildPairingPhaseStates,
  getPairingHints,
  getPairingProfile,
  normalizePairingFlow,
  resolveCurrentPairingStep,
} from './pairingSchema';

export default function PairingProgressPanel({
  activePairing,
  activePairingSession,
  selectedProtocol,
  pairingConfig,
  fallbackPairingProfile,
  sessionStatus,
  sessionStage,
  pairingNotice,
  pairingError,
  secondsRemaining,
  onStop,
  stopPending,
  onResolveNeedsInput,
}) {
  const pairingProtocol = activePairing?.protocol || selectedProtocol;
  const pairingProfileForView = getPairingProfile(pairingProtocol, pairingConfig) || fallbackPairingProfile;
  const pairingFlowForView = normalizePairingFlow(pairingProfileForView?.flow);
  const pairingHints = getPairingHints(pairingProfileForView);
  const pairingStages = useMemo(
    // Prefer the fine-grained stage signal when it's available so step cards
    // advance with adapter-reported progress stages.
    () => buildPairingPhaseStates(sessionStage || sessionStatus, pairingFlowForView),
    [sessionStage, sessionStatus, pairingFlowForView],
  );
  const currentStep = useMemo(
    () => resolveCurrentPairingStep(sessionStage || sessionStatus, pairingFlowForView),
    [sessionStage, sessionStatus, pairingFlowForView],
  );
  // Show stage label when available and fall back to human-readable status.
  const statusLabel = (sessionStage || sessionStatus || 'in_progress')
    .replace(/_/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
  const pairingMeta = activePairingSession?.metadata;
  const pairingMessage = activePairingSession?.message || activePairingSession?.progress?.message || '';
  const requiredInputs = Array.isArray(activePairingSession?.requiredInputs)
    ? activePairingSession.requiredInputs
    : [];
  const pairingDeviceId = activePairingSession?.deviceId || activePairingSession?.device_id;
  const fallbackPairingNotice = pairingProfileForView?.notes || 'Permit join active — reset the device and keep it near the coordinator.';

  return (
    <div className="auth-modal-content-inner add-device-step add-device-pairing-step">
      <div className="add-device-pairing-panel">
        <div className="add-device-pairing-runtime-header">
          <span className="add-device-pairing-step-kicker">Live pairing progress</span>
          <h5>{statusLabel}</h5>
          <p>Keep the device powered and close to the coordinator while this flow completes.</p>
          <div className="add-device-pairing-stage-breadcrumb" aria-label="Current pairing stage">
            <span>Stage {Math.max((currentStep?.index ?? 0) + 1, 1)} of {currentStep?.total || pairingStages.length}</span>
            <strong>{currentStep?.step?.label || 'Starting'}</strong>
          </div>
        </div>

        <div className="add-device-pairing-progress">
          <div className="add-device-pairing-stages">
            {pairingStages.map((phase, index) => (
              <div key={phase.id} className={`add-device-pairing-stage-card ${phase.state}`}>
                <span className="add-device-pairing-stage-index">{index + 1}</span>
                <div className="add-device-pairing-stage-body">
                  <span className="add-device-pairing-stage-label">{phase.label}</span>
                  {phase.description ? <p>{phase.description}</p> : null}
                </div>
              </div>
            ))}
          </div>

          <div className="add-device-pairing-timer">
            <div>
              <span className="add-device-pairing-label">Time left</span>
              <div className="add-device-countdown-number">{secondsRemaining ?? '—'}</div>
            </div>
            <div className="add-device-pairing-status-pill">{statusLabel}</div>
          </div>
        </div>

        <p className="add-device-pairing-note">{pairingNotice || fallbackPairingNotice}</p>

        {pairingMessage ? (
          <div className="add-device-pairing-text-block">{pairingMessage}</div>
        ) : null}

        {requiredInputs.length > 0 ? (
          <div className="add-device-pairing-meta">
            <span className="add-device-pairing-label">Required inputs</span>
            <div className="add-device-pairing-meta-grid">
              {requiredInputs.map(input => (
                <span key={input}>{`${input}`.replace(/[_-]+/g, ' ')}</span>
              ))}
            </div>
            {typeof onResolveNeedsInput === 'function' ? (
              <button
                type="button"
                className="add-device-secondary-btn"
                onClick={onResolveNeedsInput}
                disabled={stopPending}
              >
                Provide required inputs
              </button>
            ) : null}
          </div>
        ) : null}

        {pairingHints.length > 0 ? (
          <div className="add-device-pairing-inline-tips">
            {pairingHints.map(instruction => (
              <span key={instruction}>{instruction}</span>
            ))}
          </div>
        ) : null}

        {pairingMeta ? (
          <div className="add-device-pairing-meta">
            <span className="add-device-pairing-label">Metadata applied after join</span>
            <div className="add-device-pairing-meta-grid">
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
          <button type="button" className="auth-modal-btn" onClick={onStop} disabled={stopPending}>
            {stopPending ? 'Stopping…' : 'Stop pairing'}
          </button>
        </div>
      </div>
    </div>
  );
}
