import React, { useState, useEffect, useRef } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Home from './components/Home';
import Devices from './components/Devices';
import Map from './components/Map';
import Spotify from './components/Spotify';
import Profile from './components/Profile';
import './App.css';
import { isPermanentSidebarWidth } from './breakpoints.js';

export default function App() {
  const [windowWidth, setWindowWidth] = useState(window.innerWidth);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const isPermanentSidebar = isPermanentSidebarWidth(windowWidth);
  const prevIsPermanent = useRef(isPermanentSidebar);

  useEffect(() => {
    const handleResize = () => setWindowWidth(window.innerWidth);
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  useEffect(() => {
    if (!prevIsPermanent.current && isPermanentSidebar) {
      setSidebarOpen(false);
    }
    prevIsPermanent.current = isPermanentSidebar;
  }, [isPermanentSidebar]);

  // Only pass what Sidebar needs; let Sidebar handle remounting if needed
  return (
    <div>
      <Sidebar
        menuOpen={isPermanentSidebar || sidebarOpen}
        setMenuOpen={setSidebarOpen}
        isPermanentSidebar={isPermanentSidebar}
      />
      {!isPermanentSidebar && !sidebarOpen && (
        <button
          className="menu-btn"
          style={{ position: 'fixed', top: 16, left: 16, zIndex: 200 }}
          onClick={() => setSidebarOpen(true)}
          aria-label="Open menu"
        >
          <span style={{ fontSize: 28, color: '#2f3c49' }}>☰</span>
        </button>
      )}
      <main
        style={{
          flex: 1,
          minWidth: 0,
          position: 'relative',
          zIndex: 1,
          marginLeft: isPermanentSidebar ? 'calc(320px + 2.5rem)' : 0,
          transition: 'margin-left 0.3s cubic-bezier(.4,2,.6,1)',
        }}
      >
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
