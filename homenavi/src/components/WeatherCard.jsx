import React from 'react';
import GlassCard from './GlassCard';

export default function WeatherCard() {
  return (
    <GlassCard style={{ height: 320 }}>
      <div className="card-content-col">
        <div className="card-content-icon">
          <svg width="90" height="90" viewBox="0 0 90 90" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="65" cy="32" r="22" fill="#FFD93B" />
            <ellipse cx="35" cy="65" rx="25" ry="15" fill="#e5e4e3" />
            <ellipse cx="60" cy="75" rx="16" ry="9" fill="#cfd8dc" />
          </svg>
        </div>
        <div className="card-content-details">
          <span className="text-5xl font-extrabold text-white leading-tight">22°C</span>
          <span className="text-2xl font-semibold text-white">Budapest</span>
          <span className="text-lg text-white/80">Sunny</span>
        </div>
      </div>
    </GlassCard>
  );
}
