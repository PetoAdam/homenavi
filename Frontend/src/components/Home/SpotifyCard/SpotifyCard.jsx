import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBackwardStep, faForwardStep, faPlay, faPause } from '@fortawesome/free-solid-svg-icons';
import { faSpotify as faSpotifyBrand } from '@fortawesome/free-brands-svg-icons';
import GlassCard from '../../common/GlassCard/GlassCard';
import './SpotifyCard.css';

export default function SpotifyCard() {
  // Mock song data
  const [playing, setPlaying] = useState(true);
  const song = {
    title: 'Blinding Lights',
    artist: 'The Weeknd',
    progress: 42, // seconds
    duration: 200, // seconds
  };
  return (
    <GlassCard className="spotify-card">
      <div className="spotify-cc-vertical">
        <div className="spotify-cc-header">
          <FontAwesomeIcon icon={faSpotifyBrand} size="2x" className="spotify-cc-logo" />
          <span className="spotify-cc-label">Spotify</span>
        </div>
        <div className="spotify-cc-songinfo">
          <span className="spotify-cc-title">{song.title}</span>
          <span className="spotify-cc-artist">{song.artist}</span>
        </div>
        <div className="spotify-cc-slider">
          <div className="spotify-cc-slider-bar" style={{ width: `${(song.progress / song.duration) * 100}%` }}></div>
        </div>
        <div className="spotify-cc-controls">
          <button className="spotify-cc-btn" title="Previous">
            <FontAwesomeIcon icon={faBackwardStep} />
          </button>
          <button className="spotify-cc-btn play" title={playing ? 'Pause' : 'Play'} onClick={() => setPlaying(p => !p)}>
            <FontAwesomeIcon icon={playing ? faPause : faPlay} />
          </button>
          <button className="spotify-cc-btn" title="Next">
            <FontAwesomeIcon icon={faForwardStep} />
          </button>
        </div>
      </div>
    </GlassCard>
  );
}
