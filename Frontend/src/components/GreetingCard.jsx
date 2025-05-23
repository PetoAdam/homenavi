import React from 'react';
import GlassCard from './GlassCard';

export default function GreetingCard() {
  return (
    <GlassCard>
      <div className="flex flex-col items-start w-full">
        <span className="text-2xl font-bold text-[#fff] mb-1">Welcome back, Adam!</span>
        <span className="text-[#7c848d]">Have a great smart home day. 😊</span>
      </div>
    </GlassCard>
  );
}
