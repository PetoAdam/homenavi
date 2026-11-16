/**
 * Shared device control rendering components.
 * Used by both DeviceTile (Devices page) and DeviceWidget (Dashboard).
 */
import React, { useId, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBolt,
  faChevronDown,
  faChevronUp,
  faGaugeHigh,
  faMinus,
  faPalette,
  faPlus,
  faSliders,
  faTicket,
} from '@fortawesome/free-solid-svg-icons';
import GlassSwitch from '../GlassSwitch/GlassSwitch';
import GlassSelect from '../GlassSelect/GlassSelect';
import ColorPickerControl from '../ColorPickerControl/ColorPickerControl';
import { isValidControlOverride, resolveControlComponent, resolveInputLabel, sanitizeInputKey, toControlBoolean } from './deviceControlUtils';
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

function ControlLabel({ icon, label, displayValue, annotation }) {
  return (
    <div className="dcr-control-label">
      <FontAwesomeIcon icon={icon} />
      <span className="dcr-control-label-copy">
        <span className="dcr-control-label-text">{label}</span>
        {annotation ? <span className="dcr-control-annotation">{annotation}</span> : null}
      </span>
      {displayValue !== '' && displayValue !== undefined && displayValue !== null ? <span className="dcr-control-value">{displayValue}</span> : null}
    </div>
  );
}

// ────────────────────────────────────────────────────────────────────
// Control Renderer Components
// ────────────────────────────────────────────────────────────────────

function ToggleControl({ input, value, pending, onChange, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.toggle;
  const label = resolveInputLabel(input);
  const displayValue = mixed ? '' : formatControlValue(input, value);

  return (
    <div className={`dcr-control${mixed ? ' dcr-control-mixed' : ''}`} data-key={key}>
      <ControlLabel icon={icon} label={label} displayValue={displayValue} annotation={annotation} />
      <GlassSwitch
        checked={Boolean(value)}
        disabled={pending}
        mixed={mixed}
        onChange={onChange}
      />
    </div>
  );
}

function SliderControl({ input, value, pending, onChange, onCommit, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.slider;
  const label = resolveInputLabel(input);
  const dirty = useRef(false);
  const pendingValue = useRef(null);
  const range = input.range || {};
  const min = typeof range?.min === 'number' ? range.min : 0;
  const max = typeof range?.max === 'number' ? range.max : 255;
  const step = typeof range?.step === 'number' ? range.step : 1;
  const safeValue = typeof value === 'number' ? value : min;
  const fillPercent = min === max ? 0 : Math.max(0, Math.min(100, ((safeValue - min) / (max - min)) * 100));
  const missing = value === '' || value === null || value === undefined;
  const commit = () => {
    if (!dirty.current) return;
    dirty.current = false;
    const next = pendingValue.current;
    pendingValue.current = null;
    if (!isExcludedNumber(next, range)) onCommit(next);
  };

  return (
    <div className={`dcr-control dcr-control-wide${mixed ? ' dcr-control-mixed' : ''}`} data-key={key}>
      <ControlLabel
        icon={icon}
        label={label}
        displayValue={mixed || missing ? '' : normalizeAlignedNumber(value, min, step)}
        annotation={annotation || (missing ? 'No reported value' : undefined)}
      />
      <div className="dcr-control-slider">
        <input
          type="range"
          min={min}
          max={max}
          step={step}
          value={safeValue}
          disabled={pending}
          onChange={e => {
            const proposed = normalizeAlignedNumber(Number(e.target.value), min, step);
            const next = isExcludedNumber(proposed, range) ? steppedValue(safeValue, proposed > safeValue ? 1 : -1, min, max, step, range) : proposed;
            dirty.current = next !== safeValue;
            pendingValue.current = dirty.current ? next : null;
            if (dirty.current) onChange(next);
          }}
          onMouseUp={e => commit(e.currentTarget)}
          onTouchEnd={e => commit(e.currentTarget)}
          onKeyUp={e => { if (e.key === 'Enter') commit(e.currentTarget); }}
          onBlur={e => commit(e.currentTarget)}
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

function SelectControl({ input, value, pending, onChange, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.select;
  const label = resolveInputLabel(input);

  const selectOptions = useMemo(() => {
    const raw = Array.isArray(input?.options) ? input.options : [];
    return raw
      .map((option) => {
        const rawValue = option?.value;
        const stringValue = rawValue === undefined || rawValue === null ? '' : String(rawValue);
        if (!stringValue) return null;
        return {
          value: stringValue,
          label: option?.label ? String(option.label) : stringValue,
        };
      })
      .filter(Boolean);
  }, [input?.options]);

  const selectedValue = value === undefined || value === null || value === ''
    ? (selectOptions[0]?.value || '')
    : String(value);

  return (
    <div className={`dcr-control${mixed ? ' dcr-control-mixed' : ''}`} data-key={key}>
      <ControlLabel icon={icon} label={label} annotation={annotation} />
      <GlassSelect
        className="dcr-control-select"
        value={selectedValue}
        disabled={pending}
        options={selectOptions}
        placeholder="Select…"
        ariaLabel={label}
        onChange={(next) => onChange(next)}
      />
    </div>
  );
}

function isOnOffSelectInput(input) {
  const options = Array.isArray(input?.options) ? input.options : [];
  let hasOn = false;
  let hasOff = false;
  options.forEach((opt) => {
    const v = String(opt?.value ?? '').trim().toLowerCase();
    if (v === 'on') hasOn = true;
    if (v === 'off') hasOff = true;
  });
  return hasOn && hasOff;
}

function getOnOffValues(input) {
  const options = Array.isArray(input?.options) ? input.options : [];
  let onValue = 'on';
  let offValue = 'off';
  options.forEach((opt) => {
    const raw = opt?.value;
    const normalized = String(raw ?? '').trim().toLowerCase();
    if (normalized === 'on') onValue = raw;
    if (normalized === 'off') offValue = raw;
  });
  return { onValue, offValue };
}

function OnOffSelectControl({ input, value, pending, onChange, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.toggle;
  const label = resolveInputLabel(input);
  const { onValue, offValue } = useMemo(() => getOnOffValues(input), [input]);
  const checked = toControlBoolean(value);

  return (
    <div className={`dcr-control${mixed ? ' dcr-control-mixed' : ''}`} data-key={key}>
      <ControlLabel icon={icon} label={label} displayValue={mixed ? '' : (checked ? 'On' : 'Off')} annotation={annotation} />
      <GlassSwitch
        checked={checked}
        disabled={pending}
        mixed={mixed}
        onChange={(next) => onChange(next ? onValue : offValue)}
      />
    </div>
  );
}

function isExcludedNumber(value, range) {
  return Array.isArray(range?.except) && range.except.some(excluded => Number(excluded) === value);
}

function decimalPlaces(value) {
  const [mantissa, exponent = '0'] = String(value).toLowerCase().split('e');
  return Math.max(0, (mantissa.split('.')[1]?.length || 0) - Number(exponent));
}

function steppedValue(current, direction, min, max, step, range) {
  const base = min ?? 0;
  const position = (current - base) / step;
  let index = direction > 0 ? Math.floor(position + 1e-9) + 1 : Math.ceil(position - 1e-9) - 1;
	if (min !== undefined) index = Math.max(0, index);
	if (max !== undefined) index = Math.min(Math.floor((max - base) / step + 1e-9), index);
  const precision = Math.min(100, Math.max(decimalPlaces(base), decimalPlaces(step)));
  const exclusions = Array.isArray(range?.except) ? range.except.length : 0;
  for (let attempt = 0; attempt <= exclusions; attempt++) {
    if (min !== undefined && index < 0) return current;
    if (max !== undefined && index > Math.floor((max - base) / step + 1e-9)) return current;
    const next = Number((base + index * step).toFixed(precision));
    if (!isExcludedNumber(next, range)) return next;
    index += direction;
  }
  return current;
}

function isStepAligned(value, min, step) {
  const position = (value - (min ?? 0)) / step;
  return Number.isFinite(position) && Number.isSafeInteger(Math.round(position)) && Math.abs(position - Math.round(position)) <= Number.EPSILON * Math.max(1, Math.abs(position)) * 4;
}

function normalizeAlignedNumber(value, min, step) {
  if (typeof value !== 'number' || !Number.isFinite(value) || !Number.isFinite(step) || step <= 0 || !isStepAligned(value, min, step)) return value;
  const precision = Math.min(100, Math.max(decimalPlaces(min ?? 0), decimalPlaces(step)));
  return Number(value.toFixed(precision));
}

function isValidNumericField(field, min, max, step, range) {
  if (!field || field.value === '' || field.validity.badInput) return false;
  const value = field.valueAsNumber;
  return Number.isFinite(value) && (min === undefined || value >= min) && (max === undefined || value <= max) && isStepAligned(value, min, step) && !isExcludedNumber(value, range);
}

function NumberControl({ input, value, pending, onChange, onCommit, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.number;
  const label = resolveInputLabel(input);
  const dirty = useRef(false);
  const fieldRef = useRef(null);
  const range = input.range || {};
  const min = Number.isFinite(range.min) ? range.min : undefined;
  const max = Number.isFinite(range.max) ? range.max : undefined;
  const step = Number.isFinite(range.step) && range.step > 0 ? range.step : 1;
  const current = value === '' || value === null || value === undefined ? NaN : Number(value);
  const valid = Number.isFinite(current);
  const unit = input.capability?.unit || input.unit;
  const displayValue = dirty.current ? value : normalizeAlignedNumber(value, min, step);
  const commit = (field) => {
    if (!dirty.current || !isValidNumericField(field, min, max, step, range)) return;
    dirty.current = false;
    onCommit(field.value);
  };
  const adjust = direction => {
    if (!valid || mixed) return;
    const next = steppedValue(current, direction, min, max, step, range);
    if (next === current) return;
    dirty.current = false;
    onChange(next);
    onCommit(next);
  };

  return (
    <div className={`dcr-control${mixed ? ' dcr-control-mixed' : ''}`} data-key={key}>
      <ControlLabel icon={icon} label={label} annotation={annotation} />
      <div className="dcr-number-field" onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) commit(fieldRef.current); }}>
        <input
          ref={fieldRef}
          type="number"
          aria-label={label}
          className="dcr-control-number"
          min={min}
          max={max}
          step={step}
          value={mixed && !dirty.current ? '' : displayValue ?? ''}
          placeholder={mixed ? 'Mixed' : undefined}
          disabled={pending}
          onChange={e => {
            dirty.current = true;
            const raw = e.target.value;
            if (raw === '') {
              onChange('');
              return;
            }
            const numeric = Number(raw);
            if (!Number.isNaN(numeric)) onChange(numeric);
          }}
          onKeyUp={e => { if (e.key === 'Enter') commit(e.currentTarget); }}
        />
        {unit ? <span className="dcr-number-unit">{unit}</span> : null}
        <div className="dcr-number-arrows">
          <button type="button" title={`Increase ${label}`} aria-label={`Increase ${label}`} disabled={pending || mixed || !valid || (max !== undefined && current >= max)} onClick={() => adjust(1)}><FontAwesomeIcon icon={faChevronUp} /></button>
          <button type="button" title={`Decrease ${label}`} aria-label={`Decrease ${label}`} disabled={pending || mixed || !valid || (min !== undefined && current <= min)} onClick={() => adjust(-1)}><FontAwesomeIcon icon={faChevronDown} /></button>
        </div>
      </div>
    </div>
  );
}

function StepperControl({ input, value, pending, onChange, onCommit, annotation, mixed }) {
  const label = resolveInputLabel(input);
  const dirty = useRef(false);
  const fieldRef = useRef(null);
  const range = input.range || {};
  const min = Number.isFinite(range.min) ? range.min : undefined;
  const max = Number.isFinite(range.max) ? range.max : undefined;
  const step = Number.isFinite(range.step) && range.step > 0 ? range.step : 1;
  const current = value === '' || value === null || value === undefined ? NaN : Number(value);
  const valid = Number.isFinite(current);
  const displayValue = dirty.current ? value : normalizeAlignedNumber(value, min, step);
  const adjust = (direction) => {
    if (!valid || mixed) return;
    const bounded = steppedValue(current, direction, min, max, step, range);
    if (bounded === current) return;
    dirty.current = false;
    onChange(bounded);
    onCommit(bounded);
  };
  const commit = (field) => {
    if (!dirty.current || !isValidNumericField(field, min, max, step, range)) return;
    const next = field.valueAsNumber;
    if (!Number.isFinite(next)) return;
    dirty.current = false;
    onChange(next);
    onCommit(next);
  };

  return (
    <div className={`dcr-control dcr-control-wide${mixed ? ' dcr-control-mixed' : ''}`} data-key={sanitizeInputKey(input)}>
      <ControlLabel icon={faGaugeHigh} label={label} annotation={annotation} />
      <div className="dcr-stepper" onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) commit(fieldRef.current); }}>
        <button type="button" title={`Decrease ${label}`} aria-label={`Decrease ${label}`} disabled={pending || mixed || !valid || (min !== undefined && current <= min)} onClick={() => adjust(-1)}><FontAwesomeIcon icon={faMinus} /></button>
        <div className="dcr-number-field dcr-stepper-value">
          <input ref={fieldRef} type="number" aria-label={label} min={min} max={max} step={step} value={mixed && !dirty.current ? '' : displayValue ?? ''} style={{ width: `${Math.max(3, String(displayValue ?? '').length + 1)}ch` }} disabled={pending} onChange={event => { dirty.current = true; onChange(event.target.value === '' ? '' : Number(event.target.value)); }} onKeyUp={event => { if (event.key === 'Enter') commit(event.currentTarget); }} />
          {input.capability?.unit || input.unit ? <span className="dcr-number-unit">{input.capability?.unit || input.unit}</span> : null}
        </div>
        <button type="button" title={`Increase ${label}`} aria-label={`Increase ${label}`} disabled={pending || mixed || !valid || (max !== undefined && current >= max)} onClick={() => adjust(1)}><FontAwesomeIcon icon={faPlus} /></button>
      </div>
    </div>
  );
}

function CheckboxControl({ input, value, pending, onChange, annotation, mixed }) {
  const label = resolveInputLabel(input);
  return (
    <label className="dcr-control" data-key={sanitizeInputKey(input)}>
      <ControlLabel icon={faBolt} label={label} annotation={annotation} />
      <input type="checkbox" aria-label={label} aria-checked={mixed ? 'mixed' : undefined} ref={element => { if (element) element.indeterminate = mixed; }} checked={!mixed && toControlBoolean(value)} disabled={pending} onChange={event => onChange(event.target.checked)} />
    </label>
  );
}

function ChoiceControl({ input, value, pending, onChange, annotation, mixed, component }) {
  const label = resolveInputLabel(input);
  const groupName = useId();
  const options = Array.isArray(input.options) ? input.options : [];
  return (
    <div className={`dcr-control dcr-control-wide${mixed ? ' dcr-control-mixed' : ''}`} data-key={sanitizeInputKey(input)}>
      <ControlLabel icon={faTicket} label={label} annotation={annotation} />
      <div className={`dcr-choices dcr-choices-${component}`} role="radiogroup" aria-label={label}>
        {options.map(option => (
          <label key={String(option.value)}>
            <input type="radio" name={groupName} checked={!mixed && String(value) === String(option.value)} disabled={pending} onChange={() => onChange(option.value)} />
            <span>{option.label || String(option.value)}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

function ActionButtonsControl({ input, pending, onCommit, annotation }) {
  const label = resolveInputLabel(input);
  return (
    <div className="dcr-control dcr-control-wide" data-key={sanitizeInputKey(input)}>
      <ControlLabel icon={faTicket} label={label} annotation={annotation} />
      <div className="dcr-choices" role="group" aria-label={label}>
        {input.options.map(option => <button className="dcr-action-button" type="button" key={option.value} disabled={pending} onClick={() => onCommit(option.value)}>{option.label || option.value}</button>)}
      </div>
    </div>
  );
}

function TextControl({ input, value, pending, onChange, onCommit, annotation, component, mixed }) {
  const dirty = useRef(false);
  const label = resolveInputLabel(input);
  const commit = () => {
    if (!dirty.current) return;
    dirty.current = false;
    onCommit(value ?? '');
  };
  const props = {
    'aria-label': label,
    className: 'dcr-text-input',
    disabled: pending,
    value: mixed && !dirty.current ? '' : value ?? '',
    placeholder: mixed ? 'Mixed' : undefined,
    onChange: event => { dirty.current = true; onChange(event.target.value); },
    onBlur: commit,
    onKeyUp: event => { if (component === 'text' && event.key === 'Enter') commit(); },
  };
  return (
    <div className="dcr-control dcr-control-wide" data-key={sanitizeInputKey(input)}>
      <ControlLabel icon={faTicket} label={label} annotation={annotation} />
      {component === 'textarea' ? <textarea {...props} /> : <input type="text" {...props} />}
    </div>
  );
}

function ColorControl({ input, value, pending, onChange, onCommit, annotation, mixed }) {
  const key = sanitizeInputKey(input);
  const icon = ICON_BY_INPUT_TYPE.color;
  const label = resolveInputLabel(input);
  return (
    <ColorPickerControl
      containerClassName={`dcr-control dcr-control-wide${mixed ? ' dcr-control-mixed' : ''}`}
      labelRowClassName="dcr-control-label"
      label={label}
      annotation={annotation}
      icon={icon}
      value={value}
      pending={pending}
      dataKey={key}
      mixed={mixed}
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

  const annotation = input.annotation || '';
  const mixed = input.mixed === true;

  switch (resolveControlComponent(input)) {
  case 'checkbox':
    return <CheckboxControl input={input} value={value} pending={pending} annotation={annotation} mixed={mixed} onChange={next => { onChange(next); onCommit(next); }} />;
  case 'stepper':
    return <StepperControl input={input} value={value} pending={pending} annotation={annotation} mixed={mixed} onChange={onChange} onCommit={onCommit} />;
  case 'buttons':
    return <ActionButtonsControl input={input} pending={pending} annotation={annotation} onCommit={onCommit} />;
  case 'text':
  case 'textarea':
    return <TextControl input={input} value={value} pending={pending} annotation={annotation} mixed={mixed} component={resolveControlComponent(input)} onChange={onChange} onCommit={onCommit} />;
  case 'segmented':
  case 'radio': {
    const component = resolveControlComponent(input);
    return <ChoiceControl input={input} value={value} pending={pending} annotation={annotation} mixed={mixed} component={component} onChange={next => { onChange(next); onCommit(next); }} />;
  }
  case 'toggle':
    return (
      <ToggleControl
        input={input}
        value={toControlBoolean(value)}
        pending={pending}
        annotation={annotation}
        mixed={mixed}
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
        annotation={annotation}
        mixed={mixed}
        onChange={onChange}
        onCommit={onCommit}
      />
    );
  case 'select':
    if (!isValidControlOverride(input) && isOnOffSelectInput(input)) {
      return (
        <OnOffSelectControl
          input={input}
          value={value}
          pending={pending}
          annotation={annotation}
          mixed={mixed}
          onChange={(next) => {
            onChange(next);
            onCommit(next);
          }}
        />
      );
    }
    return (
      <SelectControl
        input={input}
        value={value}
        pending={pending}
        annotation={annotation}
        mixed={mixed}
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
        annotation={annotation}
        mixed={mixed}
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
        annotation={annotation}
        mixed={mixed}
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
