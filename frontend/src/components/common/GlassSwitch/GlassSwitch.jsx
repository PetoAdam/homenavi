// GlassSwitch.jsx
import React from 'react';
import './GlassSwitch.css';

export default function GlassSwitch({ checked, onChange, disabled, mixed = false }) {
  return (
    <button
      className={`glass-switch${checked ? ' checked' : ''}${disabled ? ' disabled' : ''}${mixed ? ' mixed' : ''}`}
      onClick={() => !disabled && onChange(!checked)}
      aria-pressed={checked}
      aria-disabled={disabled}
      aria-label={mixed ? 'Mixed state' : undefined}
      tabIndex={0}
      type="button"
    >
      <span className="glass-switch-track">
        <span className="glass-switch-thumb" />
      </span>
    </button>
  );
}
