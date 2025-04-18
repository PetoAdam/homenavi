import React from 'react';

export default function MenuItem({ icon, label, active, onClick }) {
  return (
    <button
      className={`menu-item${active ? ' active' : ''}`}
      onClick={onClick}
      tabIndex={0}
      type="button"
    >
      <span className="menu-icon" aria-hidden>{icon}</span>
      <span>{label}</span>
    </button>
  );
}
