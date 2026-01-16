import React, { useCallback, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HexColorPicker } from 'react-colorful';
import { normalizeColorHex } from '../../../utils/colorHex';
import './ColorPickerControl.css';

export default function ColorPickerControl({
  containerClassName,
  labelRowClassName,
  label,
  icon,
  value,
  pending,
  dataKey,
  onChange,
  onCommit,
}) {
  const [isActive, setIsActive] = useState(false);
  const [draft, setDraft] = useState(null);

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
        <span>{label}</span>
        <span className="hn-color-value">{normalizedValue}</span>
      </div>

      <div className="hn-color-summary">
        <button
          type="button"
          className="hn-color-toggle"
          onClick={() => (isActive ? closePicker() : openPicker())}
          disabled={pending}
        >
          <span className="hn-color-swatch" style={{ backgroundColor: normalizedValue }} />
          <span>{isActive ? 'Close picker' : 'Adjust color'}</span>
        </button>
      </div>

      {isActive && (
        <div className="hn-color-popover">
          <HexColorPicker color={normalizedValue} onChange={setDraft} />
          <div className="hn-color-actions">
            <input
              type="text"
              className="hn-color-input"
              value={normalizedValue}
              onChange={(e) => setDraft(e.target.value)}
            />
            <div className="hn-color-buttons">
              <button type="button" className="hn-color-cancel" onClick={closePicker}>Cancel</button>
              <button
                type="button"
                className="hn-color-apply"
                onClick={applyDraft}
                disabled={pending}
              >
                Apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
