import React from 'react';
import './GlassCard.css';

export default function GlassCard({ children, className = '', interactive = true }) {
  return (
  <div className={`glass-card ${interactive ? '' : 'no-hover'} ${className}`}>
      <div className="card-content">
        {children}
      </div>
    </div>
  );
}
