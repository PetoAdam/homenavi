import React from 'react';
import WeatherCard from './WeatherCard';
import DeviceCard from './DeviceCard';
import SpotifyCard from './SpotifyCard';
import TempHumidityCard from './TempHumidityCard';
import MapCard from './MapCard';
import AddDeviceCard from './AddDeviceCard';
import MasonryDashboard from './MasonryDashboard';
import Greeting from './Greeting';
import ProfileButton from './ProfileButton';

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
