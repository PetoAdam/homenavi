/**
 * Shared device control rendering components.
 * Used by both DeviceTile (Devices page) and DeviceWidget (Dashboard).
 */
import React, { useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBolt,
  faGaugeHigh,
  faPalette,
  faSliders,
  faTicket,
} from '@fortawesome/free-solid-svg-icons';
import GlassSwitch from '../GlassSwitch/GlassSwitch';
import ColorPickerControl from '../ColorPickerControl/ColorPickerControl';
import { resolveInputLabel, sanitizeInputKey, toControlBoolean } from './deviceControlUtils';
import './DeviceControlRenderer.css';

// ────────────────────────────────────────────────────────────────────
// Helper utilities (shared logic extracted from DeviceTile)
// ────────────────────────────────────────────────────────────────────

const ICON_BY_INPUT_TYPE = {
  toggle: faBolt,
  slider: faSliders,
  number: faGaugeHigh,
  select: faTicket,
  color: faPalette,
};

function formatControlValue(input, value) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  switch (input.type) {
  case 'toggle':
    return value ? 'On' : 'Off';
  case 'color':
    return String(value).toUpperCase();
  default:
    return value;
  }
}

// ────────────────────────────────────────────────────────────────────
// Control Renderer Components
// ────────────────────────────────────────────────────────────────────

function ToggleControl({ input, value, pending, onChange }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.toggle;
  const label = resolveInputLabel(input);
  const displayValue = formatControlValue(input, value);

  return (
    <div className="dcr-control" data-key={key}>
      <div className="dcr-control-label">
        <FontAwesomeIcon icon={icon} />
        <span>{label}</span>
        {displayValue && <span className="dcr-control-value">{displayValue}</span>}
      </div>
      <GlassSwitch
        checked={Boolean(value)}
        disabled={pending}
        onChange={onChange}
      />
    </div>
  );
}

function SliderControl({ input, value, pending, onChange, onCommit }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.slider;
  const label = resolveInputLabel(input);
  const range = input.range || {};
  const min = typeof range?.min === 'number' ? range.min : 0;
  const max = typeof range?.max === 'number' ? range.max : 255;
  const step = typeof range?.step === 'number' ? range.step : 1;
  const safeValue = typeof value === 'number' ? value : min;
  const fillPercent = min === max ? 0 : Math.max(0, Math.min(100, ((safeValue - min) / (max - min)) * 100));

  return (
    <div className="dcr-control dcr-control-wide" data-key={key}>
      <div className="dcr-control-label">
        <FontAwesomeIcon icon={icon} />
        <span>{label}</span>
        <span className="dcr-control-value">{Math.round(value ?? min)}</span>
      </div>
      <div className="dcr-control-slider">
        <input
          type="range"
          min={min}
          max={max}
          step={step}
          value={safeValue}
          disabled={pending}
          onChange={e => onChange(Number(e.target.value))}
          onMouseUp={e => onCommit(Number(e.target.value))}
          onTouchEnd={e => onCommit(Number(e.target.value))}
          onKeyUp={e => { if (e.key === 'Enter') onCommit(Number(e.target.value)); }}
          onBlur={e => onCommit(Number(e.target.value))}
          className="dcr-control-range"
          style={{ '--dcr-fill': `${fillPercent}%` }}
        />
        <div className="dcr-control-slider-scale">
          <span>{min}</span>
          <span>{max}</span>
        </div>
      </div>
    </div>
  );
}

function SelectControl({ input, value, pending, onChange }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.select;
  const label = resolveInputLabel(input);

  return (
    <div className="dcr-control" data-key={key}>
      <div className="dcr-control-label">
        <FontAwesomeIcon icon={icon} />
        <span>{label}</span>
      </div>
      <select
        className="dcr-control-select"
        value={value ?? (input.options?.[0]?.value || '')}
        disabled={pending}
        onChange={e => onChange(e.target.value)}
      >
        {(input.options || []).map(option => (
          <option key={option.value} value={option.value}>
            {option.label || option.value}
          </option>
        ))}
      </select>
    </div>
  );
}

function NumberControl({ input, value, pending, onChange, onCommit }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.number;
  const label = resolveInputLabel(input);
  const range = input.range || {};
  const min = typeof range?.min === 'number' ? range.min : undefined;
  const max = typeof range?.max === 'number' ? range.max : undefined;
  const step = typeof range?.step === 'number' ? range.step : 1;

  return (
    <div className="dcr-control" data-key={key}>
      <div className="dcr-control-label">
        <FontAwesomeIcon icon={icon} />
        <span>{label}</span>
      </div>
      <input
        type="number"
        className="dcr-control-number"
        min={min}
        max={max}
        step={step}
        value={value === '' ? '' : value ?? ''}
        disabled={pending}
        onChange={e => {
          const raw = e.target.value;
          if (raw === '') {
            onChange('');
            return;
          }
          const numeric = Number(raw);
          if (!Number.isNaN(numeric)) {
            onChange(numeric);
          }
        }}
        onBlur={e => onCommit(e.target.value)}
        onKeyUp={e => { if (e.key === 'Enter') onCommit(e.target.value); }}
      />
    </div>
  );
}

function ColorControl({ input, value, pending, onChange, onCommit }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.color;
  const label = resolveInputLabel(input);
  return (
    <ColorPickerControl
      containerClassName="dcr-control dcr-control-wide"
      labelRowClassName="dcr-control-label"
      label={label}
      icon={icon}
      value={value}
      pending={pending}
      dataKey={key}
      onChange={onChange}
      onCommit={onCommit}
    />
  );
}

// ────────────────────────────────────────────────────────────────────
// Main Renderer
// ────────────────────────────────────────────────────────────────────

/**
 * Renders a single device control based on input type.
 * @param {Object} props
 * @param {Object} props.input - The input definition object
 * @param {*} props.value - Current control value
 * @param {boolean} props.pending - Whether a command is pending
 * @param {Function} props.onChange - Called when value changes (for immediate UI feedback)
 * @param {Function} props.onCommit - Called when value should be sent to device (command)
 */
export function DeviceControlRenderer({ input, value, pending, onChange, onCommit }) {
  if (!input) return null;

  switch (input.type) {
  case 'toggle':
    return (
      <ToggleControl
        input={input}
        value={toControlBoolean(value)}
        pending={pending}
        onChange={next => {
          onChange(toControlBoolean(next));
          onCommit(toControlBoolean(next));
        }}
      />
    );
  case 'slider':
    return (
      <SliderControl
        input={input}
        value={value}
        pending={pending}
        onChange={onChange}
        onCommit={onCommit}
      />
    );
  case 'select':
    return (
      <SelectControl
        input={input}
        value={value}
        pending={pending}
        onChange={val => {
          onChange(val);
          onCommit(val);
        }}
      />
    );
  case 'number':
    return (
      <NumberControl
        input={input}
        value={value}
        pending={pending}
        onChange={onChange}
        onCommit={val => {
          if (val !== '' && val !== null && val !== undefined) {
            const numeric = Number(val);
            if (!Number.isNaN(numeric)) onCommit(numeric);
          }
        }}
      />
    );
  case 'color':
    return (
      <ColorControl
        input={input}
        value={value}
        pending={pending}
        onChange={onChange}
        onCommit={onCommit}
      />
    );
  default:
    return null;
  }
}

/**
 * Renders a list of device controls.
 * @param {Object} props
 * @param {Array} props.inputs - Array of input definitions
 * @param {Object} props.values - Map of input key -> current value
 * @param {boolean} props.pending - Whether a command is pending
 * @param {Function} props.onValueChange - (key, newValue) => void
 * @param {Function} props.onCommand - (input, value) => Promise
 * @param {string} props.layout - 'cards' or 'list'
 * @param {number} props.collapseAfter - Number of controls before collapsing (default 5)
 */
export default function DeviceControlList({
  inputs = [],
  values = {},
  pending = false,
  onValueChange,
  onCommand,
  layout = 'cards',
  collapseAfter = 5,
}) {
  const [showAll, setShowAll] = useState(false);
  const hasExtra = inputs.length > collapseAfter;
  const visibleInputs = useMemo(
    () => (showAll ? inputs : inputs.slice(0, collapseAfter)),
    [inputs, showAll, collapseAfter],
  );
  const hiddenCount = Math.max(inputs.length - collapseAfter, 0);

  if (inputs.length === 0) return null;

  const handleChange = (input, val) => {
    const key = sanitizeInputKey(input);
    if (key && onValueChange) onValueChange(key, val);
  };

  const handleCommit = (input, val) => {
    if (onCommand) onCommand(input, val);
  };

  // Stop propagation to prevent parent onClick handlers from firing
  const handleControlClick = (e) => {
    e.stopPropagation();
  };

  return (
    <div 
      className={`dcr-control-list dcr-layout-${layout}`}
      onClick={handleControlClick}
      onKeyDown={(e) => e.stopPropagation()}
    >
      {visibleInputs.map(input => {
        const key = sanitizeInputKey(input);
        const value = values[key];
        return (
          <DeviceControlRenderer
            key={key}
            input={input}
            value={value}
            pending={pending}
            onChange={val => handleChange(input, val)}
            onCommit={val => handleCommit(input, val)}
          />
        );
      })}
      {hasExtra && (
        <button
          type="button"
          className="dcr-controls-toggle"
          onClick={() => setShowAll(prev => !prev)}
        >
          {showAll ? 'Show fewer controls' : `Show ${hiddenCount} more`}
        </button>
      )}
    </div>
  );
}
