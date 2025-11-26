import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './GlassPill.css';

export default function GlassPill({ icon, text, tone = 'default', title, className = '', children, onClick }) {
  const Component = onClick ? 'button' : 'span';
  return (
    <Component
      type={onClick ? 'button' : undefined}
      className={`glass-pill glass-pill-${tone} ${onClick ? 'glass-pill-clickable' : ''} ${className}`.trim()}
      title={title}
      onClick={onClick}
    >
      {icon && <FontAwesomeIcon icon={icon} className="glass-pill-icon" />}
      <span>{text || children}</span>
    </Component>
  );
}
