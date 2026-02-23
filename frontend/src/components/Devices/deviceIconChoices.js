import {
  faBatteryThreeQuarters,
  faBolt,
  faBroom,
  faCloud,
  faCloudRain,
  faCloudSun,
  faDoorOpen,
  faDroplet,
  faFan,
  faFire,
  faGear,
  faHouse,
  faKey,
  faLightbulb,
  faLock,
  faMicrochip,
  faMusic,
  faPlug,
  faSatelliteDish,
  faShieldHalved,
  faSnowflake,
  faThermometerHalf,
  faVideo,
  faWandMagicSparkles,
  faWind,
} from '@fortawesome/free-solid-svg-icons';

export const DEVICE_ICON_CHOICES = [
  { key: 'auto', label: 'Automatic', icon: faWandMagicSparkles },
  { key: 'home', label: 'Home', icon: faHouse },
  { key: 'light', label: 'Lights', icon: faLightbulb },
  { key: 'plug', label: 'Outlet', icon: faPlug },
  { key: 'switch', label: 'Switch', icon: faBolt },
  { key: 'thermostat', label: 'Climate', icon: faThermometerHalf },
  { key: 'cooling', label: 'Cooling', icon: faSnowflake },
  { key: 'wind', label: 'Wind', icon: faWind },
  { key: 'weather', label: 'Weather', icon: faCloudSun },
  { key: 'rain', label: 'Rain', icon: faCloudRain },
  { key: 'cloud', label: 'Cloud', icon: faCloud },
  { key: 'sensor', label: 'Sensor', icon: faMicrochip },
  { key: 'door', label: 'Door', icon: faDoorOpen },
  { key: 'lock', label: 'Lock', icon: faLock },
  { key: 'water', label: 'Water', icon: faDroplet },
  { key: 'fire', label: 'Fire', icon: faFire },
  { key: 'battery', label: 'Battery', icon: faBatteryThreeQuarters },
  { key: 'security', label: 'Security', icon: faShieldHalved },
  { key: 'camera', label: 'Camera', icon: faVideo },
  { key: 'fan', label: 'Fan', icon: faFan },
  { key: 'audio', label: 'Audio', icon: faMusic },
  { key: 'antenna', label: 'Antenna', icon: faSatelliteDish },
  { key: 'cleaning', label: 'Cleaning', icon: faBroom },
  { key: 'settings', label: 'Settings', icon: faGear },
  { key: 'key', label: 'Key', icon: faKey },
];

export const DEVICE_ICON_MAP = DEVICE_ICON_CHOICES.reduce((acc, choice) => {
  if (choice.key !== 'auto') {
    acc[choice.key] = choice.icon;
  }
  return acc;
}, {});
