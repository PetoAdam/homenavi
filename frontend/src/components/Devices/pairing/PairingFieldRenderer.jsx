import React from 'react';

function normalizeValue(value, field) {
  if (field.component === 'number') {
    if (value === '') return '';
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : '';
  }
  return value;
}

function FieldWrap({ field, children }) {
  const isInput = !['text_block', 'loading'].includes(field.component);
  return (
    <div className="add-device-pairing-field">
      {isInput ? <label className="add-device-pairing-field-label" htmlFor={`pairing-field-${field.id}`}>{field.label}</label> : null}
      {children}
      {field.description ? <small className="add-device-modal-hint">{field.description}</small> : null}
    </div>
  );
}

function TextField({ field, value, onChange }) {
  const type = field.type === 'password' ? 'password' : 'text';
  return (
    <FieldWrap field={field}>
      <input
        id={`pairing-field-${field.id}`}
        className="auth-modal-input"
        type={type}
        value={value ?? ''}
        placeholder={field.placeholder || ''}
        required={field.required}
        onChange={event => onChange(field.id, event.target.value)}
      />
    </FieldWrap>
  );
}

function NumberField({ field, value, onChange }) {
  return (
    <FieldWrap field={field}>
      <input
        id={`pairing-field-${field.id}`}
        className="auth-modal-input"
        type="number"
        min={field.min ?? undefined}
        max={field.max ?? undefined}
        step={field.step ?? undefined}
        value={value ?? ''}
        required={field.required}
        onChange={event => onChange(field.id, normalizeValue(event.target.value, field))}
      />
    </FieldWrap>
  );
}

function CheckboxField({ field, value, onChange }) {
  return (
    <FieldWrap field={field}>
      <label className="add-device-pairing-checkbox" htmlFor={`pairing-field-${field.id}`}>
        <input
          id={`pairing-field-${field.id}`}
          type="checkbox"
          checked={Boolean(value)}
          onChange={event => onChange(field.id, event.target.checked)}
        />
        <span>{field.placeholder || field.label}</span>
      </label>
    </FieldWrap>
  );
}

function SelectField({ field, value, onChange }) {
  const multiple = field.multiple || field.component === 'multiselect';
  const current = multiple ? (Array.isArray(value) ? value : []) : `${value ?? ''}`;
  return (
    <FieldWrap field={field}>
      <select
        id={`pairing-field-${field.id}`}
        className="auth-modal-input add-device-select"
        value={current}
        multiple={multiple}
        required={field.required}
        onChange={event => {
          if (multiple) {
            const selected = Array.from(event.target.selectedOptions).map(option => option.value);
            onChange(field.id, selected);
            return;
          }
          onChange(field.id, event.target.value);
        }}
      >
        {!multiple ? <option value="">Select…</option> : null}
        {field.options.map(option => (
          <option key={`${field.id}-${option.value}`} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </FieldWrap>
  );
}

function SelectorField({ field, value, onChange }) {
  const multiple = field.multiple || field.component === 'list_selector';
  const selectedValues = multiple ? (Array.isArray(value) ? value : []) : [`${value ?? ''}`];
  return (
    <FieldWrap field={field}>
      <div className={`add-device-pairing-selector ${field.component === 'card_selector' ? 'cards' : 'list'}`}>
        {field.options.map(option => {
          const active = selectedValues.includes(option.value);
          return (
            <button
              key={`${field.id}-${option.value}`}
              type="button"
              className={`add-device-pairing-selector-item${active ? ' active' : ''}`}
              onClick={() => {
                if (multiple) {
                  if (active) {
                    onChange(field.id, selectedValues.filter(item => item !== option.value));
                    return;
                  }
                  onChange(field.id, [...selectedValues, option.value]);
                  return;
                }
                onChange(field.id, option.value);
              }}
            >
              <strong>{option.label}</strong>
              {option.description ? <span>{option.description}</span> : null}
            </button>
          );
        })}
      </div>
    </FieldWrap>
  );
}

function QRPayloadField({ field, value, onChange }) {
  return (
    <FieldWrap field={field}>
      <textarea
        id={`pairing-field-${field.id}`}
        className="auth-modal-input add-device-textarea"
        rows={3}
        value={value ?? ''}
        placeholder={field.placeholder || 'Paste QR payload (e.g. MT:...)'}
        onChange={event => onChange(field.id, event.target.value)}
      />
      <small className="add-device-modal-hint">QR camera scanner preset is available as a UI slot; this build uses paste/manual input.</small>
    </FieldWrap>
  );
}

function TextBlockField({ field }) {
  return (
    <FieldWrap field={field}>
      <div className="add-device-pairing-text-block">
        {field.label}
      </div>
    </FieldWrap>
  );
}

function LoadingField({ field }) {
  return (
    <FieldWrap field={field}>
      <div className="add-device-pairing-loading">
        <span className="add-device-pairing-spinner" aria-hidden="true" />
        <span>{field.label}</span>
      </div>
    </FieldWrap>
  );
}

export default function PairingFieldRenderer({ field, value, onChange }) {
  switch (field.component) {
    case 'number':
      return <NumberField field={field} value={value} onChange={onChange} />;
    case 'checkbox':
      return <CheckboxField field={field} value={value} onChange={onChange} />;
    case 'select':
    case 'dropdown':
    case 'single_select':
    case 'multiselect':
      return <SelectField field={field} value={value} onChange={onChange} />;
    case 'card_selector':
    case 'list_selector':
      return <SelectorField field={field} value={value} onChange={onChange} />;
    case 'qr_payload':
    case 'qr_reader':
      return <QRPayloadField field={field} value={value} onChange={onChange} />;
    case 'text_block':
    case 'text':
      if (field.type === 'text' && !field.options?.length && !field.bind && field.label && !field.placeholder) {
        return <TextBlockField field={field} />;
      }
      return <TextField field={field} value={value} onChange={onChange} />;
    case 'loading':
      return <LoadingField field={field} />;
    case 'password':
      return <TextField field={{ ...field, type: 'password' }} value={value} onChange={onChange} />;
    default:
      return <TextField field={field} value={value} onChange={onChange} />;
  }
}
