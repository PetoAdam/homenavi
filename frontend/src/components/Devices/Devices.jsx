import React from 'react';
import GlassCard from '../common/GlassCard/GlassCard';

export default function Devices() {
  return (
    <div className="p-6">
      <h2 className="text-xl font-bold mb-6 text-white">Devices</h2>
      <div className="grid gap-8 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <GlassCard>
          <div className="font-semibold text-white mb-2">Zigbee Sensor</div>
          <div className="text-gray-200 text-sm">Temperature: 21°C</div>
        </GlassCard>
        <GlassCard>
          <div className="font-semibold text-white mb-2">Smart Plug</div>
          <div className="text-gray-200 text-sm">Power: 12W</div>
        </GlassCard>
        {/* Add more device cards as needed */}
      </div>
    </div>
  );
}
