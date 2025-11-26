import React, { useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faPuzzlePiece, faChevronDown } from '@fortawesome/free-solid-svg-icons';
import { DEVICE_ICON_CHOICES } from './deviceIconChoices';
import './AddDeviceModal.css';

const CAPABILITIES_EXAMPLE = '[{ "id": "state", "kind": "binary" }]';
const INPUTS_EXAMPLE = '[{ "type": "toggle", "property": "state" }]';

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
  } catch (err) {
    throw new Error(`${label} must be valid JSON`);
  }
}

function buildIdentifier(protocol, name, manual) {
  if (manual && manual.trim()) {
    return manual.trim();
  }
  const base = slugify(name) || `device-${Date.now().toString(36)}`;
  const safeProtocol = protocol || 'manual';
  return `${safeProtocol}/${base}`;
}

export default function AddDeviceModal({
  open,
  onClose,
  onCreate,
  integrations = [],
}) {
  const [form, setForm] = useState(defaultForm);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const modalRef = useRef(null);

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
  const selectedIcon = form.icon || 'auto';
  const identifierPreview = useMemo(() => buildIdentifier(selectedProtocol, form.name, form.identifier), [selectedProtocol, form.name, form.identifier]);

  useEffect(() => {
    if (!open) return undefined;
    const handleKey = event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose?.();
      }
    };
    document.addEventListener('keydown', handleKey);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKey);
      document.body.style.overflow = '';
    };
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return undefined;
    const handleClick = event => {
      if (!modalRef.current) return;
      if (!modalRef.current.contains(event.target)) {
        onClose?.();
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open, onClose]);

  useEffect(() => {
    if (!open) {
      setForm(defaultForm);
      setShowAdvanced(false);
      setError(null);
    }
  }, [open]);

  if (!open) {
    return null;
  }

  const handleSubmit = async event => {
    event.preventDefault();
    if (!onCreate) return;
    if (!selectedProtocol) {
      setError('Select a protocol');
      return;
    }
    try {
      setSubmitting(true);
      setError(null);
      const payload = {
        protocol: selectedProtocol.trim().toLowerCase(),
        external_id: identifierPreview,
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
      onClose?.();
      setForm(defaultForm);
    } catch (err) {
      setSubmitting(false);
      setError(err?.message || 'Unable to create device');
    }
  };

  const handleBackdropMouseDown = event => {
    if (event.target === event.currentTarget) {
      onClose?.();
    }
  };

  return (
    <div
      className={`auth-modal-backdrop add-device-modal-backdrop${open ? ' open' : ''}`}
      onMouseDown={handleBackdropMouseDown}
    >
      <div
        className={`auth-modal-glass add-device-modal-glass${open ? ' open' : ''}`}
        ref={modalRef}
      >
        <button type="button" className="auth-modal-close" onClick={onClose} aria-label="Close add device dialog">
          ×
        </button>

        <div className="auth-modal-content">
          <div className="auth-modal-header add-device-modal-header">
            <FontAwesomeIcon icon={faPuzzlePiece} className="auth-modal-avatar add-device-modal-avatar" />
            <span className="auth-modal-title">Add Device</span>
            <p className="auth-modal-subtitle">Provide protocol info and optional metadata. All fields can be edited later.</p>
          </div>
          <div className="auth-modal-content-outer">
            <div className="auth-modal-content-inner login">
              <form className="auth-modal-form add-device-form" onSubmit={handleSubmit} noValidate>
                <div className={`auth-modal-field add-device-field add-device-field-select${selectedProtocol ? ' filled' : ''}`}>
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
                    onChange={event => setForm(prev => ({ ...prev, identifier: event.target.value }))}
                  />
                  <label className="auth-modal-label" htmlFor="add-device-identifier">Identifier (optional)</label>
                  <small className="add-device-modal-hint">Preview: {identifierPreview}</small>
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
                <div className="add-device-modal-note">
                  These basics get your device registered. Need manufacturer details, descriptions, firmware or JSON? Pop open Advanced metadata below.
                </div>

                <button
                  type="button"
                  className={`add-device-advanced-toggle${showAdvanced ? ' open' : ''}`}
                  onClick={() => setShowAdvanced(prev => !prev)}
                  aria-expanded={showAdvanced}
                >
                  <span>Advanced metadata</span>
                  <FontAwesomeIcon icon={faChevronDown} />
                </button>

                <div className={`add-device-advanced${showAdvanced ? ' open' : ''}`}>
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
                  <div className="add-device-row">
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
                  </div>
                  <div className="auth-modal-field add-device-field add-device-description-field">
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
                  <div className="auth-modal-field add-device-field">
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
                  <div className="auth-modal-field add-device-field">
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

                {error ? <div className="auth-modal-error add-device-error">{error}</div> : null}
                <button className="auth-modal-btn" type="submit" disabled={submitting}>
                  {submitting ? 'Creating…' : 'Create device'}
                </button>
              </form>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
