import React from 'react';
import GlassCard from './GlassCard';
import './MapCard.css';

export default function MapCard() {
  return (
    <GlassCard>
      <div className="relative w-full h-full flex flex-col justify-between items-center">
        {/* Blueprint/Map SVG (replace with your own SVG or PNG if needed) */}
        <img src="https://upload.wikimedia.org/wikipedia/commons/3/3c/House_blueprint_sample.png" alt="Apartment Blueprint" className="map-card-svg" />
        <div className="map-card-bottom">
          <span className="map-card-label">MAP</span>
          <button className="map-card-btn">&gt;</button>
        </div>
      </div>
    </GlassCard>
  );
}
