import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSliders } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../common/GlassCard/GlassCard';
import { getControlPresentationOptions, getDisplayPresentationOptions, resolveInputLabel, sanitizeInputKey } from '../common/DeviceControlRenderer/deviceControlUtils';
import { patchErsDevice } from '../../services/entityRegistryService';
import { buildPresentationInputs } from '../../utils/groupControls';

const LABELS = {
  toggle: 'Switch', checkbox: 'Checkbox', slider: 'Slider', number: 'Number', stepper: 'Stepper',
  select: 'Dropdown', segmented: 'Segmented', radio: 'Radio', buttons: 'Action buttons', color: 'Color picker',
  text: 'Text', textarea: 'Text area', metric: 'Metric', badge: 'Badge', progress: 'Progress', gauge: 'Gauge',
};

export default function DevicePresentationSettings({ device, accessToken, refresh }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const controls = buildPresentationInputs(device).map(input => ({
    key: sanitizeInputKey(input), label: resolveInputLabel(input), options: getControlPresentationOptions(input),
  })).filter(item => item.key && item.options.length);
  const seen = new Set();
  const displays = [...(Array.isArray(device?.capabilities) ? device.capabilities : []), ...(Array.isArray(device?.state?.capabilities) ? device.state.capabilities : [])]
    .map(capability => ({
      key: capability?.id || capability?.property || capability?.name,
      label: capability?.name || capability?.property || capability?.id,
      options: getDisplayPresentationOptions(capability),
    }))
    .filter(item => {
      if (!item.key || !item.options.length || seen.has(item.key)) return false;
      seen.add(item.key);
      return true;
    });

  async function save(kind, key, value) {
    if (!accessToken || !device?.ersId) {
      setError('Device metadata is unavailable or authentication is required.');
      return;
    }
    setSaving(true);
    setError('');
    try {
      const result = await patchErsDevice(device.ersId, { meta: { presentation: { [kind]: { [key]: value || null } } } }, accessToken);
      if (!result.success) throw new Error(result.error || 'Unable to save presentation');
      await refresh();
    } catch (err) {
      setError(err?.message || 'Unable to save presentation');
    } finally {
      setSaving(false);
    }
  }

  function renderItems(title, kind, items) {
    if (!items.length) return null;
    return (
      <div className="device-presentation-section">
        <div className="device-history-field-label">{title}</div>
        <div className="device-presentation-options">
          {items.map(item => {
            const stored = device?.meta?.presentation?.[kind]?.[item.key];
            return (
              <label className="device-history-field" key={item.key}>
                <span>{item.label}</span>
                <select aria-label={`${title}: ${item.label}`} disabled={saving} value={item.options.includes(stored) ? stored : ''} onChange={event => void save(kind, item.key, event.target.value)}>
                  <option value="">Integration default</option>
                  {item.options.map(option => <option key={option} value={option}>{LABELS[option] || option}</option>)}
                </select>
              </label>
            );
          })}
        </div>
      </div>
    );
  }

  return (
    <GlassCard className="device-ers-meta-card" interactive={false}>
      <div className="device-history-header">
        <div className="device-history-title"><span className="device-history-icon"><FontAwesomeIcon icon={faSliders} /></span><span>Presentation</span></div>
      </div>
      {error ? <div className="device-history-error" role="alert">{error}</div> : null}
      {!device?.ersId ? <div className="device-history-empty-text">Device metadata unavailable.</div> : (
        <div className="device-history-controls device-presentation-controls">
          {renderItems('Controls', 'controls', controls)}
          {renderItems('State displays', 'displays', displays)}
          {!controls.length && !displays.length ? <div className="device-history-empty-text">No compatible presentation options.</div> : null}
        </div>
      )}
    </GlassCard>
  );
}