import React, { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';
import './RoleSelect.css';

export default function RoleSelect({ value, options, disabled, onChange, saving }) {
  const [open, setOpen] = useState(false);
  const [dropUp, setDropUp] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, left: 0, width: 0 });
  const ref = useRef(null);
  const menuRef = useRef(null);
  useEffect(() => {
    const handler = (e) => {
      // If click is inside trigger container or inside the portal menu, ignore
      if (ref.current?.contains(e.target) || menuRef.current?.contains(e.target)) return;
      setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);
  // Compute and set menu position (pre-open and after mount for accuracy)
  const computePosition = (willOpen = true) => {
    if (!ref.current) return;
    const trigger = ref.current.querySelector('.role-select-trigger');
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const viewportH = window.innerHeight;
    const estHeight = Math.min(options.length * 34 + 20, 260);
    // Decide dropUp before opening
    const spaceBelow = viewportH - rect.bottom;
    const useDropUp = spaceBelow < estHeight + 12;
    setDropUp(useDropUp);
    const top = useDropUp ? Math.max(8, rect.top - estHeight - 8) : Math.min(rect.bottom + 4, viewportH - 8);
    const left = Math.min(Math.max(8, rect.left), window.innerWidth - rect.width - 8);
    setMenuPos({ top, left, width: rect.width });
    if (willOpen) {
      setOpen(true);
      setFocusIndex(options.findIndex(o => o === value));
      setTimeout(() => { menuRef.current?.focus(); }, 0);
    }
  };

  useEffect(() => {
    if (!open) return;
    // Re-measure actual height after render to adjust precise top for dropUp
    const adjust = () => {
      if (!menuRef.current) return;
      const h = menuRef.current.getBoundingClientRect().height;
      const trigger = ref.current?.querySelector('.role-select-trigger');
      if (!trigger) return;
      const rect = trigger.getBoundingClientRect();
      const viewportH = window.innerHeight;
      const spaceBelow = viewportH - rect.bottom;
      const needDropUp = spaceBelow < h + 12;
      if (needDropUp !== dropUp) setDropUp(needDropUp);
      const top = needDropUp ? Math.max(8, rect.top - h - 8) : Math.min(rect.bottom + 4, viewportH - 8);
      const left = Math.min(Math.max(8, rect.left), window.innerWidth - rect.width - 8);
      setMenuPos({ top, left, width: rect.width });
    };
    requestAnimationFrame(adjust);
    const listeners = ['scroll', 'resize'];
    const handler = () => adjust();
    listeners.forEach(ev => window.addEventListener(ev, handler, true));
    return () => listeners.forEach(ev => window.removeEventListener(ev, handler, true));
  }, [open, options.length, dropUp]);
  const current = options.find(o => o === value) || value;
  const [focusIndex, setFocusIndex] = useState(() => options.findIndex(o => o === value));
  useEffect(() => { setFocusIndex(options.findIndex(o => o === value)); }, [value, options]);
  return (
    <div className={`role-select${disabled ? ' disabled' : ''}${open ? ' open' : ''}${dropUp ? ' drop-up' : ''}`} ref={ref}>
      <button
        type="button"
        className="role-select-trigger"
        disabled={disabled}
        onClick={() => {
          if (open) { setOpen(false); return; }
          computePosition(true);
        }}
        aria-haspopup="listbox"
        aria-expanded={open}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            if (!open) computePosition(true); else setFocusIndex(f => Math.min(options.length - 1, (f === -1 ? 0 : f)));
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            if (!open) computePosition(true);
          }
        }}
      >
        <span>{current}</span>
  {saving ? <span className="role-select-spinner" aria-label="Saving" /> : <span className="caret" aria-hidden="true">▾</span>}
      </button>
      {open && !disabled && createPortal(
        <ul
          className={`role-select-menu portal${dropUp ? ' drop-up' : ''}`}
          role="listbox"
          ref={menuRef}
          style={{ position: 'fixed', top: menuPos.top, left: menuPos.left, minWidth: menuPos.width }}
          tabIndex={-1}
          aria-activedescendant={focusIndex >= 0 ? `role-opt-${options[focusIndex]}` : undefined}
          onKeyDown={(e) => {
            if (e.key === 'Escape') { e.preventDefault(); setOpen(false); ref.current?.querySelector('.role-select-trigger')?.focus(); }
            if (e.key === 'ArrowDown') { e.preventDefault(); setFocusIndex(i => Math.min(options.length - 1, (i + 1 + options.length) % options.length)); }
            if (e.key === 'ArrowUp') { e.preventDefault(); setFocusIndex(i => Math.max(0, (i - 1 + options.length) % options.length)); }
            if (e.key === 'Home') { e.preventDefault(); setFocusIndex(0); }
            if (e.key === 'End') { e.preventDefault(); setFocusIndex(options.length - 1); }
            if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (focusIndex >= 0) { const opt = options[focusIndex]; onChange(opt); setOpen(false); ref.current?.querySelector('.role-select-trigger')?.focus(); } }
          }}
        >
          {options.map((opt, idx) => (
            <li
              id={`role-opt-${opt}`}
              key={opt}
              role="option"
              aria-selected={opt === value}
              className={`${opt === value ? 'active' : ''} ${idx === focusIndex ? 'focus' : ''}`}
              onClick={() => { onChange(opt); setOpen(false); ref.current?.querySelector('.role-select-trigger')?.focus(); }}
              onMouseEnter={() => setFocusIndex(idx)}
            >
              {opt}
            </li>
          ))}
        </ul>,
        document.body
      )}
    </div>
  );
}
