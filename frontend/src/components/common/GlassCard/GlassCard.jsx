import React from 'react';
import './GlassCard.css';

const GlassCard = React.forwardRef(function GlassCard(
  { children, className = '', interactive = true, ...props },
  ref,
) {
  return (
    <div
      ref={ref}
      className={`glass-card ${interactive ? '' : 'no-hover'} ${className}`}
      {...props}
    >
      <div className="card-content">
        {children}
      </div>
    </div>
  );
});

export default GlassCard;
