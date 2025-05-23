import React from 'react';
import GlassCard from './GlassCard';

export default function Profile() {
  return (
    <div className="p-6">
      <h2 className="text-xl font-bold mb-6 text-white">Profile</h2>
      <div className="flex justify-center">
        <GlassCard className="w-full max-w-lg flex flex-col items-center">
          <span className="text-gray-200 mb-2">User profile placeholder</span>
          <button className="mt-4 px-4 py-2 rounded bg-blue-600 text-white font-semibold hover:bg-blue-700 transition">Logout</button>
        </GlassCard>
      </div>
    </div>
  );
}
