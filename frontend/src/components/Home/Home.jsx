import React from 'react';
import WeatherCard from './WeatherCard/WeatherCard';
import DeviceCard from './DeviceCard/DeviceCard';
import SpotifyCard from './SpotifyCard/SpotifyCard';
import TempHumidityCard from './TempHumidityCard/TempHumidityCard';
import MapCard from './MapCard/MapCard';
import AddDeviceCard from './AddDeviceCard/AddDeviceCard';
import MasonryDashboard from './MasonryDashboard/MasonryDashboard';
import Greeting from './Greeting/Greeting';

export default function Home() {
  return (
    <div className="p-6">
        <Greeting showProfileTextButton />
      <MasonryDashboard>
        <WeatherCard />
        <SpotifyCard />
        <DeviceCard />
        <TempHumidityCard />
        <MapCard />
        <AddDeviceCard />
      </MasonryDashboard>
    </div>
  );
}
