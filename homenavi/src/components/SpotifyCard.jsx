import React from 'react';
import GlassCard from './GlassCard';

export default function SpotifyCard() {
  // Mock song data
  const song = {
    title: 'Blinding Lights',
    artist: 'The Weeknd',
    progress: 42, // seconds
    duration: 200, // seconds
  };
  return (
    <GlassCard className="spotify-card">
      <div className="card-content-col">
        <div className="card-content-icon">
          <svg width="80" height="80" viewBox="0 0 80 80" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="40" cy="40" r="40" fill="#1DB954" />
            <path d="M25 55c10-5 30-5 40 0" stroke="white" strokeWidth="4" strokeLinecap="round"/>
            <path d="M30 40c8-3 24-3 32 0" stroke="white" strokeWidth="3" strokeLinecap="round"/>
            <path d="M34 28c5-2 15-2 20 0" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </svg>
        </div>
        <div className="card-content-details flex-1">
          <span className="text-3xl font-extrabold text-white leading-tight mb-1">{song.title}</span>
          <span className="text-lg text-white/80 mb-2">{song.artist}</span>
          <div className="spotify-slider">
            <div className="spotify-slider-bar" style={{ width: `${(song.progress / song.duration) * 100}%` }}></div>
          </div>
          <div className="spotify-controls mt-2">
            <button className="spotify-btn" title="Shuffle">
              <i className="fa fa-random" aria-hidden="true"></i>
            </button>
            <button className="spotify-btn" title="Previous">
              <i className="fa fa-step-backward" aria-hidden="true"></i>
            </button>
            <button className="spotify-btn" title="Play/Pause">
              <i className="fa fa-play" aria-hidden="true"></i>
            </button>
            <button className="spotify-btn" title="Next">
              <i className="fa fa-step-forward" aria-hidden="true"></i>
            </button>
            <button className="spotify-btn" title="Volume">
              <i className="fa fa-volume-up" aria-hidden="true"></i>
            </button>
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
