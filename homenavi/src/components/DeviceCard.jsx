import React, { useState } from 'react';
import GlassCard from './GlassCard';

export default function DeviceCard() {
  const [on, setOn] = useState(true);
  return (
    <GlassCard style={{ height: 260 }}>
      <div className="card-content-col">
        <div className="card-content-icon">
          <svg width="80" height="80" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
            <ellipse cx="20" cy="16" rx="8" ry="10" fill="#e5e4e3" />
            <rect x="17" y="26" width="6" height="6" rx="2" fill="#7c848d" />
            <rect x="19" y="30" width="2" height="3" rx="1" fill={on ? "#39aa79" : "#7c848d"} />
          </svg>
        </div>
        <div className="card-content-details w-full">
          <span className="text-3xl font-extrabold text-white leading-tight">Living Room Light</span>
          <span className="text-lg text-white/80">Zigbee</span>
          <div className="flex items-center gap-3 mt-4">
            <span className={`text-xl font-semibold ${on ? 'text-[#39aa79]' : 'text-[#7c848d]'}`}>{on ? 'On' : 'Off'}</span>
            <button
              className={`w-16 h-10 rounded-full flex items-center transition bg-[#e5e4e3] relative shadow-inner ml-2`}
              style={{ boxShadow: on ? '0 0 8px #39aa79' : '0 0 8px #7c848d' }}
              onClick={() => setOn(v => !v)}
            >
              <span
                className={`absolute left-1 top-1 w-8 h-8 rounded-full transition-all duration-300 ${on ? 'translate-x-6 bg-[#39aa79]' : 'translate-x-0 bg-[#7c848d]'}`}
                style={{ boxShadow: '0 2px 8px #0002' }}
              />
            </button>
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
