import React from 'react';
import GlassCard from '../../common/GlassCard/GlassCard';

export default function TempHumidityCard() {
  return (
    <GlassCard style={{height: 260}}>
      <div className="card-content-col">
        <div className="card-content-icon">
          <svg width="80" height="80" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
            <ellipse cx="20" cy="20" rx="16" ry="16" fill="#e5e4e3" />
            <rect x="17" y="10" width="6" height="16" rx="3" fill="#7c848d" />
            <circle cx="20" cy="28" r="5" fill="#39aa79" />
            <rect x="23" y="13" width="2" height="6" rx="1" fill="#39aa79" />
          </svg>
        </div>
        <div className="card-content-details">
          <span className="text-4xl font-extrabold text-white leading-tight">21°C</span>
          <span className="text-2xl font-semibold text-white">48% Humidity</span>
          <span className="text-lg text-white/80">Bedroom Sensor</span>
        </div>
      </div>
    </GlassCard>
  );
}
