import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './GlassPill.css';

export default function GlassPill({ icon, text, tone = 'default', title, className = '', children }) {
  return (
    <span className={`glass-pill glass-pill-${tone} ${className}`} title={title}>
      {icon && <FontAwesomeIcon icon={icon} className="glass-pill-icon" />}
      <span>{text || children}</span>
    </span>
  );
}
