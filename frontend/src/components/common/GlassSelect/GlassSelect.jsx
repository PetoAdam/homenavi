import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCheck, faChevronDown } from '@fortawesome/free-solid-svg-icons';
import './GlassSelect.css';

function normalizeOptions(options) {
  const list = Array.isArray(options) ? options : [];
  return list
    .map((opt) => {
      if (typeof opt === 'string') return { value: opt, label: opt };
      if (!opt || typeof opt !== 'object') return null;
      const value = typeof opt.value === 'string' ? opt.value : '';
      const label = typeof opt.label === 'string' ? opt.label : value;
      if (!value) return null;
      return { value, label };
    })
    .filter(Boolean);
}

export default function GlassSelect({
  value,
  onChange,
  options,
  disabled = false,
  className = '',
  placeholder = 'Select…',
  ariaLabel,
}) {
  const rootRef = useRef(null);
  const menuRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [menuPos, setMenuPos] = useState(null);

  const normalized = useMemo(() => normalizeOptions(options), [options]);
  const selected = useMemo(() => normalized.find((o) => o.value === value) || null, [normalized, value]);

  const close = useCallback(() => setOpen(false), []);

  const recomputeMenuPosition = useCallback(() => {
    const root = rootRef.current;
    if (!root) return;
    const trigger = root.querySelector('.glass-select__trigger');
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    setMenuPos({
      left: rect.left,
      top: rect.bottom + 8,
      width: rect.width,
      // used for optional flip logic later
      triggerTop: rect.top,
    });
  }, []);

  useEffect(() => {
    if (!open) return undefined;
    recomputeMenuPosition();

    const onDocMouseDown = (e) => {
      const el = rootRef.current;
      const menuEl = menuRef.current;
      if (!el) return;
      if (el.contains(e.target)) return;
      if (menuEl && menuEl.contains(e.target)) return;
      setOpen(false);
    };
    document.addEventListener('mousedown', onDocMouseDown);
    const onWin = () => recomputeMenuPosition();
    window.addEventListener('resize', onWin);
    window.addEventListener('scroll', onWin, true);

    return () => {
      document.removeEventListener('mousedown', onDocMouseDown);
      window.removeEventListener('resize', onWin);
      window.removeEventListener('scroll', onWin, true);
    };
  }, [open, recomputeMenuPosition]);

  return (
    <div
      ref={rootRef}
      className={`glass-select ${disabled ? 'glass-select--disabled' : ''} ${className}`.trim()}
    >
      <button
        type="button"
        className="glass-select__trigger"
        onClick={() => {
          if (disabled) return;
          setOpen((v) => {
            const next = !v;
            if (next) {
              // compute position after state applies
              requestAnimationFrame(recomputeMenuPosition);
            }
            return next;
          });
        }}
        disabled={disabled}
        aria-label={ariaLabel}
        aria-expanded={open}
      >
        <span className={`glass-select__value ${selected ? '' : 'glass-select__value--placeholder'}`.trim()}>
          {selected ? selected.label : placeholder}
        </span>
        <FontAwesomeIcon icon={faChevronDown} className={`glass-select__chev ${open ? 'open' : ''}`} />
      </button>

      {open && menuPos && createPortal(
        <div
          ref={menuRef}
          className="glass-select__menu glass-select__menu--portal"
          role="listbox"
          style={{ left: `${menuPos.left}px`, top: `${menuPos.top}px`, width: `${menuPos.width}px` }}
        >
          {normalized.length === 0 ? (
            <div className="glass-select__empty">No options</div>
          ) : (
            normalized.map((opt) => {
              const isActive = opt.value === value;
              return (
                <button
                  key={opt.value}
                  type="button"
                  className={`glass-select__option ${isActive ? 'active' : ''}`.trim()}
                  onClick={() => {
                    if (disabled) return;
                    onChange?.(opt.value);
                    close();
                  }}
                  role="option"
                  aria-selected={isActive}
                  title={opt.label}
                >
                  <span className="glass-select__option-label">{opt.label}</span>
                  {isActive && <FontAwesomeIcon icon={faCheck} className="glass-select__check" />}
                </button>
              );
            })
          )}
        </div>,
        document.body,
      )}
    </div>
  );
}
