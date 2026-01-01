import React, { useMemo } from 'react';
import './ChipMultiSelect.css';

function normalizeOptions(options, getLabel) {
  const list = Array.isArray(options) ? options : [];
  return list
    .map((opt) => {
      if (typeof opt === 'string') {
        return { value: opt, label: getLabel ? getLabel(opt) : opt };
      }
      if (opt && typeof opt === 'object') {
        const value = typeof opt.value === 'string' ? opt.value : '';
        const label = typeof opt.label === 'string' ? opt.label : (getLabel ? getLabel(value) : value);
        return value ? { value, label } : null;
      }
      return null;
    })
    .filter(Boolean);
}

export default function ChipMultiSelect({
  options,
  value,
  onChange,
  disabled = false,
  className = '',
  ariaLabel,
  emptyText = 'No options',
  selectedFirst = true,
  getLabel,
}) {
  const selected = useMemo(() => new Set(Array.isArray(value) ? value : []), [value]);

  const normalized = useMemo(() => {
    const list = normalizeOptions(options, getLabel);
    if (!selectedFirst) return list;
    return list.slice().sort((a, b) => {
      const aSel = selected.has(a.value) ? 1 : 0;
      const bSel = selected.has(b.value) ? 1 : 0;
      if (aSel !== bSel) return bSel - aSel;
      return a.label.localeCompare(b.label, undefined, { sensitivity: 'base' });
    });
  }, [getLabel, options, selected, selectedFirst]);

  const toggle = (optValue) => {
    if (disabled) return;
    const current = Array.isArray(value) ? value : [];
    const set = new Set(current);
    if (set.has(optValue)) {
      set.delete(optValue);
    } else {
      set.add(optValue);
    }
    const next = Array.from(set);
    onChange?.(next);
  };

  if (normalized.length === 0) {
    return <div className={`chip-multi-select-empty ${className}`.trim()}>{emptyText}</div>;
  }

  return (
    <div
      className={`chip-multi-select-row ${disabled ? 'chip-multi-select-disabled' : ''} ${className}`.trim()}
      role="group"
      aria-label={ariaLabel}
    >
      {normalized.map(opt => {
        const active = selected.has(opt.value);
        return (
          <button
            key={opt.value}
            type="button"
            className={`chip-multi-select-chip chip-multi-select-btn${active ? ' active' : ''}`}
            onClick={() => toggle(opt.value)}
            disabled={disabled}
            title={opt.label}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
