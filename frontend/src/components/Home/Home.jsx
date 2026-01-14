import React from 'react';
import Dashboard from './Dashboard/Dashboard';
import Greeting from './Greeting/Greeting';

export default function Home() {
  return (
    <div className="p-6">
      <Greeting showProfileTextButton />
      <Dashboard />
    </div>
  );
}
