import React, { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HexColorPicker } from 'react-colorful';
import { normalizeColorHex } from '../../../utils/colorHex';
import './ColorPickerControl.css';

function ColorPickerDialog({ label, draft, value, pending, onDraftChange, onClose, onApply, triggerRef }) {
  const dialogRef = useRef(null);
  const titleId = useId();
  const valid = /^#?[0-9a-f]{6}$/i.test(draft.trim());
  const color = valid ? normalizeColorHex(draft) : normalizeColorHex(value);

  useEffect(() => {
    const dialog = dialogRef.current;
    const trigger = triggerRef.current;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    if (typeof dialog?.showModal === 'function') dialog.showModal();
    else dialog?.setAttribute('open', '');
    return () => {
      if (dialog?.open && typeof dialog.close === 'function') dialog.close();
      document.body.style.overflow = overflow;
      trigger?.focus();
    };
  }, [triggerRef]);

  return createPortal(
    <dialog
      ref={dialogRef}
      className="hn-color-dialog"
      aria-labelledby={titleId}
      onKeyDown={event => event.stopPropagation()}
      onCancel={event => { event.preventDefault(); event.stopPropagation(); onClose(); }}
      onClick={event => { event.stopPropagation(); if (event.target === event.currentTarget) onClose(); }}
    >
      <div className="hn-color-dialog-content">
        <div className="hn-color-dialog-header">
          <div><span className="hn-color-dialog-eyebrow">Choose color</span><h2 id={titleId}>{label}</h2></div>
          <span className="hn-color-dialog-preview" style={{ backgroundColor: color }} />
        </div>
        <div className="hn-color-popover">
          <HexColorPicker color={color} onChange={onDraftChange} />
          <div className="hn-color-actions">
            <label className="hn-color-input-label">Hex color
              <input type="text" className="hn-color-input" value={draft} aria-invalid={!valid} onChange={event => onDraftChange(event.target.value)} />
            </label>
            <div className="hn-color-buttons">
              <button type="button" className="hn-color-cancel" onClick={onClose}>Cancel</button>
              <button type="button" className="hn-color-apply" onClick={onApply} disabled={pending || !valid}>Apply</button>
            </div>
          </div>
        </div>
      </div>
    </dialog>,
    document.body,
  );
}

export default function ColorPickerControl({
  containerClassName,
  labelRowClassName,
  label,
  annotation,
  icon,
  value,
  pending,
  dataKey,
  mixed = false,
  onChange,
  onCommit,
}) {
  const [isActive, setIsActive] = useState(false);
  const [draft, setDraft] = useState(null);
  const triggerRef = useRef(null);

  const normalizedValue = useMemo(() => normalizeColorHex(draft ?? value), [draft, value]);

  const openPicker = useCallback(() => {
    if (pending) return;
    setIsActive(true);
    setDraft(normalizeColorHex(value ?? '#FFFFFF'));
  }, [pending, value]);

  const closePicker = useCallback(() => {
    setIsActive(false);
    setDraft(null);
  }, []);

  const applyDraft = useCallback(() => {
    const finalHex = normalizeColorHex(draft ?? value ?? '#FFFFFF');
    onChange?.(finalHex);
    onCommit?.(finalHex);
    closePicker();
  }, [draft, value, onChange, onCommit, closePicker]);

  return (
    <div
      className={`${containerClassName} hn-color-control${isActive ? ' hn-color-active' : ''}`.trim()}
      data-key={dataKey}
    >
      <div className={labelRowClassName}>
        {icon && <FontAwesomeIcon icon={icon} />}
        <span className="hn-color-label-copy">
          <span className="hn-color-label-text">{label}</span>
          {annotation ? <span className="hn-color-annotation">{annotation}</span> : null}
        </span>
        {!mixed ? <span className="hn-color-value">{normalizedValue}</span> : null}
      </div>

      <div className="hn-color-summary">
        <button
          ref={triggerRef}
          type="button"
          className="hn-color-toggle"
          onClick={openPicker}
          disabled={pending}
          aria-haspopup="dialog"
        >
          <span className="hn-color-swatch" style={{ backgroundColor: normalizedValue }} />
          <span>Adjust color</span>
        </button>
      </div>

      {isActive && (
        <ColorPickerDialog label={label} draft={draft} value={value} pending={pending} onDraftChange={setDraft} onClose={closePicker} onApply={applyDraft} triggerRef={triggerRef} />
      )}
    </div>
  );
}
