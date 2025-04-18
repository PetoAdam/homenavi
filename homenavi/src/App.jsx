import React, { useState, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Home from './components/Home';
import Devices from './components/Devices';
import Map from './components/Map';
import Spotify from './components/Spotify';
import Profile from './components/Profile';
import './App.css';
import { BREAKPOINTS, isPermanentSidebarWidth } from './breakpoints.js';

export default function App() {
  const [sidebarOpen, setSidebarOpen] = React.useState(false);
  const [windowWidth, setWindowWidth] = React.useState(window.innerWidth);
  const isPermanentSidebar = isPermanentSidebarWidth(windowWidth);

  React.useEffect(() => {
    const handleResize = () => {
      setWindowWidth(window.innerWidth);
      if (window.innerWidth < BREAKPOINTS.permanentSidebar) setSidebarOpen(false);
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'row', minHeight: '100vh', width: '100%', overflow: 'hidden' }}>
      {isPermanentSidebar && (
        <Sidebar menuOpen={true} setMenuOpen={setSidebarOpen} />
      )}
      {!isPermanentSidebar && (
        <>
          {/* Hamburger button for mobile and medium widths */}
          {!sidebarOpen && (
            <button
              className="menu-btn"
              style={{ position: 'fixed', top: 16, left: 16, zIndex: 200, display: sidebarOpen ? 'none' : 'block' }}
              onClick={() => setSidebarOpen(true)}
              aria-label="Open menu"
            >
              <span style={{ fontSize: 28, color: '#2f3c49' }}>☰</span>
            </button>
          )}
          {/* Sidebar with close button for mobile and medium widths */}
          {sidebarOpen && (
            <Sidebar menuOpen={sidebarOpen} setMenuOpen={setSidebarOpen} />
          )}
        </>
      )}
      <main style={{ flex: 1, minWidth: 0, position: 'relative', zIndex: 1 }}>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/devices" element={<Devices />} />
          <Route path="/profile" element={<Profile />} />
          <Route path="/map" element={<Map />} />
          <Route path="/spotify" element={<Spotify />} />
          <Route path="*" element={<Navigate to="/" />} />
        </Routes>
      </main>
    </div>
  );
}
