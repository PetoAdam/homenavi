import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

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
