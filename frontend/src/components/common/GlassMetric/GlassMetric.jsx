import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './GlassMetric.css';

function formatValue(value) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'number') {
    const abs = Math.abs(value);
    const formatted = abs >= 100 ? value.toFixed(0) : value.toFixed(1);
    return formatted.replace(/\.0$/, '');
  }
  return String(value);
}

export default function GlassMetric({ icon, label, value, unit, hint, className = '' }) {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  const displayValue = formatValue(value);
  return (
    <div className={`glass-metric ${className}`}>
      <div className="glass-metric-label">
        {icon && <FontAwesomeIcon icon={icon} className="glass-metric-icon" />}
        <span>{label}</span>
      </div>
      <div className="glass-metric-value">
        <span>
          {displayValue}
          {unit ? <span className="glass-metric-unit">{unit}</span> : null}
        </span>
        {hint ? <span className="glass-metric-hint">{hint}</span> : null}
      </div>
    </div>
  );
}
