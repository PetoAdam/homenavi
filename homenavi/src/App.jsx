import React, { useState, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Home from './components/Home';
import Devices from './components/Devices';
import Map from './components/Map';
import Spotify from './components/Spotify';
import Profile from './components/Profile';
import './App.css';

export default function App() {
  const [sidebarOpen, setSidebarOpen] = React.useState(false);
  const [isMobile, setIsMobile] = React.useState(window.innerWidth < 768);

  React.useEffect(() => {
    const handleResize = () => {
      setIsMobile(window.innerWidth < 768);
      if (window.innerWidth >= 768) setSidebarOpen(false);
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'row', minHeight: '100vh', width: '100%', overflow: 'hidden' }}>
      {/* Only render sidebar and its space on desktop */}
      {!isMobile && (
        <aside style={{ flex: '0 0 18rem', minWidth: '8rem', maxWidth: '8rem', maxHeight: 'fit-content', zIndex: 100, position: 'relative', display: 'block' }}>
          <Sidebar menuOpen={true} setMenuOpen={setSidebarOpen} />
        </aside>
      )}
      {/* Hamburger button and overlay sidebar on mobile */}
      {isMobile && (
      <>
        <button
          className="menu-btn"
          style={{ position: 'fixed', top: 16, left: 16, zIndex: 200, display: sidebarOpen ? 'none' : 'block' }}
          onClick={() => setSidebarOpen(true)}
          aria-label="Open menu"
        >
          <span style={{ fontSize: 28, color: '#2f3c49' }}>☰</span>
        </button>
        <aside className={`sidebar${sidebarOpen ? ' open' : ''}`}>
          <button
            className="close-btn"
            style={{ position: 'fixed', top: 16, left: 16, zIndex: 350 }}
            onClick={() => setSidebarOpen(false)}
            aria-label="Close menu"
          >
            ✕
          </button>
          <Sidebar menuOpen={sidebarOpen} setMenuOpen={setSidebarOpen} />
        </aside>
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
