import React from 'react';
import './GlassCard.css';

export default function GlassCard({ children, className = '' }) {
  return (
    <div className={`glass-card ${className}`}>
      <div className="card-content">
        {children}
      </div>
    </div>
  );
}
