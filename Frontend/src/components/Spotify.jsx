import React from 'react';
import GlassCard from './GlassCard';

export default function Spotify() {
  return (
    <div className="p-6">
      <h2 className="text-xl font-bold mb-6 text-white">Spotify</h2>
      <div className="flex justify-center">
        <GlassCard className="w-full max-w-xl h-40 flex items-center justify-center">
          <span className="text-gray-200">Spotify integration placeholder</span>
        </GlassCard>
      </div>
    </div>
  );
}
