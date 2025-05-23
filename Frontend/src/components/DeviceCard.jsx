import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLightbulb, faCog } from '@fortawesome/free-solid-svg-icons';
import GlassCard from './GlassCard';
import GlassSwitch from './GlassSwitch';
import './DeviceCard.css';

export default function DeviceCard() {
  const [on, setOn] = useState(true);
  return (
    <GlassCard className="device-card">
      <div className="devicecard-vertical">
        <div className="devicecard-toprow">
          <div className="devicecard-iconwrap">
            <FontAwesomeIcon icon={faLightbulb} size="2x" className="devicecard-icon" style={{ color: on ? '#FFD93B' : '#7c848d' }} />
          </div>
          <div className="devicecard-details">
            <span className="devicecard-title">Living Room Light</span>
            <span className="devicecard-sub">Zigbee</span>
          </div>
        </div>
        <div className="devicecard-controls-below">
          <GlassSwitch checked={on} onChange={setOn} />
          <button className="glass-btn devicecard-btn" title="Settings">
            <FontAwesomeIcon icon={faCog} size="lg" />
          </button>
        </div>
      </div>
    </GlassCard>
  );
}
