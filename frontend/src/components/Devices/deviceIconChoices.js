import {
  faBatteryThreeQuarters,
  faBolt,
  faDoorOpen,
  faDroplet,
  faFan,
  faLightbulb,
  faMicrochip,
  faMusic,
  faPlug,
  faShieldHalved,
  faThermometerHalf,
  faVideo,
  faWandMagicSparkles,
} from '@fortawesome/free-solid-svg-icons';

export const DEVICE_ICON_CHOICES = [
  { key: 'auto', label: 'Automatic', icon: faWandMagicSparkles },
  { key: 'light', label: 'Lights', icon: faLightbulb },
  { key: 'plug', label: 'Outlet', icon: faPlug },
  { key: 'switch', label: 'Switch', icon: faBolt },
  { key: 'thermostat', label: 'Climate', icon: faThermometerHalf },
  { key: 'sensor', label: 'Sensor', icon: faMicrochip },
  { key: 'door', label: 'Door', icon: faDoorOpen },
  { key: 'water', label: 'Water', icon: faDroplet },
  { key: 'battery', label: 'Battery', icon: faBatteryThreeQuarters },
  { key: 'security', label: 'Security', icon: faShieldHalved },
  { key: 'camera', label: 'Camera', icon: faVideo },
  { key: 'fan', label: 'Fan', icon: faFan },
  { key: 'audio', label: 'Audio', icon: faMusic },
];

export const DEVICE_ICON_MAP = DEVICE_ICON_CHOICES.reduce((acc, choice) => {
  if (choice.key !== 'auto') {
    acc[choice.key] = choice.icon;
  }
  return acc;
}, {});
