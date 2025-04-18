import React from 'react';

export default function GlassCard({ children, className = '' }) {
  return (
    <div className={`glass-card ${className}`}>
      <div className="card-content">
        {children}
      </div>
    </div>
  );
}
