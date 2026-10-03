import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBolt,
  faDoorOpen,
  faFan,
  faGaugeHigh,
  faLightbulb,
  faPlug,
  faSliders,
  faTag,
  faThermometerHalf,
  faToggleOn,
  faWaveSquare,
} from '@fortawesome/free-solid-svg-icons';

const deviceIcons = {
  blinds: faSliders,
  door: faDoorOpen,
  fan: faFan,
  lightbulb: faLightbulb,
  plug: faPlug,
};

const capabilityIcons = {
  air_quality: faWaveSquare,
  battery: faBolt,
  brightness: faLightbulb,
  contact: faDoorOpen,
  fan_speed: faFan,
  humidity: faWaveSquare,
  on: faToggleOn,
  position: faSliders,
  power: faBolt,
  power_draw: faGaugeHigh,
  temperature: faThermometerHalf,
};

export function DeviceIcon({ device, title = 'Device' }) {
  const icon = deviceIcons[String(device?.icon || '').toLowerCase()] || faPlug;
  return <FontAwesomeIcon icon={icon} title={title} aria-label={title} />;
}

export function CapabilityIcon({ capability, title }) {
  const key = String(capability?.id || capability?.property || '').toLowerCase();
  const icon = capabilityIcons[key] || (capability?.type === 'numeric' ? faGaugeHigh : capability?.type === 'binary' ? faToggleOn : faTag);
  return <FontAwesomeIcon icon={icon} title={title || capability?.label || 'Capability'} aria-label={title || capability?.label || 'Capability'} />;
}