import React from 'react';
import GlassCard from './GlassCard';
import './AddDeviceCard.css';

export default function AddDeviceCard() {
  return (
    <GlassCard>
      <div className="flex flex-col items-center justify-between w-full h-full relative">
        <span className="add-device-plus">+</span>
        <span className="add-device-label">Add Device</span>
      </div>
    </GlassCard>
  );
}
