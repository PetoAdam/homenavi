import React from 'react';
import WeatherCard from './WeatherCard';
import DeviceCard from './DeviceCard';
import SpotifyCard from './SpotifyCard';
import TempHumidityCard from './TempHumidityCard';
import MapCard from './MapCard';
import AddDeviceCard from './AddDeviceCard';

export default function Home() {
  return (
    <div className="p-6">
      <div className="dashboard-greeting">
        Welcome back, Adam!👋
      </div>
      <div className="masonry-dashboard">
        <WeatherCard />
        <SpotifyCard />
        <DeviceCard />
        <TempHumidityCard />
        <MapCard />
        <AddDeviceCard />
      </div>
    </div>
  );
}
