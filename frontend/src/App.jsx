import React, { useState, useEffect, useRef } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Sidebar from './components/Sidebar/Sidebar.jsx';
import Home from './components/Home/Home.jsx';
import Devices from './components/Devices/Devices.jsx';
import DeviceDetail from './components/Devices/DeviceDetail.jsx';
import Map from './components/Map/Map.jsx';
import Spotify from './components/Spotify/Spotify.jsx';
import Users from './components/Users/Users.jsx';
import Profile from './components/Profile/Profile.jsx';
import ProfileButton from './components/common/ProfileButton/ProfileButton.jsx';
import './App.css';
import { isPermanentSidebarWidth } from './breakpoints.js';
import { AuthProvider } from './context/AuthContext';

export default function App() {
  const [windowWidth, setWindowWidth] = useState(window.innerWidth);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const isPermanentSidebar = isPermanentSidebarWidth(windowWidth);
  const prevIsPermanent = useRef(isPermanentSidebar);
  const sidebarRef = useRef();

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

  useEffect(() => {
    if (!sidebarOpen || isPermanentSidebar) return;
    function handle(e) {
      if (
        sidebarRef.current &&
        !sidebarRef.current.contains(e.target)
      ) {
        setSidebarOpen(false);
      }
    }
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [sidebarOpen, isPermanentSidebar]);

  // Only pass what Sidebar needs; let Sidebar handle remounting if needed
  return (
    <AuthProvider>
      <div>
        <Sidebar
          menuOpen={isPermanentSidebar || sidebarOpen}
          setMenuOpen={setSidebarOpen}
          isPermanentSidebar={isPermanentSidebar}
          ref={sidebarRef}
        />
        {/* Profile button in top right */}
        <div
          style={{
            position: 'fixed',
            top: 24,
            right: 36,
            zIndex: 300,
            display: 'flex',
            alignItems: 'center',
            gap: '0.7rem',
          }}
        >
          <ProfileButton />
        </div>
        {!isPermanentSidebar && !sidebarOpen && (
          <button
            className="menu-btn"
            aria-label="Open menu"
            onClick={() => setSidebarOpen(true)}
          >
            <span
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: '100%',
                height: '100%',
                fontSize: 28,
                color: 'var(--color-glass-border)',
              }}
            >
              ☰
            </span>
          </button>
        )}
        <main
          style={{
            marginTop: '2rem',
            padding: '2rem 0 2rem 0',
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
            <Route path="/devices/:deviceId" element={<DeviceDetail />} />
            <Route path="/profile" element={<Profile />} />
            <Route path="/map" element={<Map />} />
            <Route path="/spotify" element={<Spotify />} />
            <Route path="/users" element={<Users />} />
            <Route path="*" element={<Navigate to="/" />} />
          </Routes>
        </main>
      </div>
    </AuthProvider>
  );
}
