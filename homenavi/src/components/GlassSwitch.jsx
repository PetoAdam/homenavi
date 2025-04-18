// GlassSwitch.jsx
import React from 'react';
import './GlassSwitch.css';

export default function GlassSwitch({ checked, onChange, disabled }) {
  return (
    <button
      className={`glass-switch${checked ? ' checked' : ''}${disabled ? ' disabled' : ''}`}
      onClick={() => !disabled && onChange(!checked)}
      aria-pressed={checked}
      aria-disabled={disabled}
      tabIndex={0}
      type="button"
    >
      <span className="glass-switch-track">
        <span className="glass-switch-thumb" />
      </span>
    </button>
  );
}
