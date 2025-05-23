import React from 'react';
import GlassCard from './GlassCard';

export default function Map() {
  return (
    <div className="p-6">
      <h2 className="text-xl font-bold mb-6 text-white">Map</h2>
      <div className="flex justify-center">
        <GlassCard className="w-full max-w-2xl h-80 flex items-center justify-center">
          <span className="text-gray-200">Apartment map placeholder</span>
        </GlassCard>
      </div>
    </div>
  );
}
