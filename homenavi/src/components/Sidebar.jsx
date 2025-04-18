import React from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import MenuItem from './MenuItem';
import { BREAKPOINTS, isPermanentSidebarWidth } from '../breakpoints.js';

const menuItems = [
  { name: 'Home', path: '/', icon: '🏠' },
  { name: 'Devices', path: '/devices', icon: '💡' },
  { name: 'Map', path: '/map', icon: '🗺️' },
  { name: 'Spotify', path: '/spotify', icon: '🎵' },
  { name: 'Profile', path: '/profile', icon: '👤' },
];

export default function Sidebar({ menuOpen, setMenuOpen }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [windowWidth, setWindowWidth] = React.useState(window.innerWidth);
  const isPermanentSidebar = isPermanentSidebarWidth(windowWidth);

  // Use flex layout, no fixed/absolute positioning
  return (
    <aside
      className={`sidebar${!isPermanentSidebar && menuOpen ? ' open' : ''} flex flex-col justify-start`}
      // Remove all inline positioning, let parent flexbox handle layout
    >
      {!isPermanentSidebar && menuOpen && (
        <button
          className="close-btn md:hidden"
          style={{ zIndex: 350, top: 16, left: 16 }}
          onClick={() => setMenuOpen(false)}
          aria-label="Close menu"
        >
          ✕
        </button>
      )}
      <div className="flex items-center justify-between px-2 pb-2 border-b border-white/10 mb-4">
        <span className="glass-logo select-none">homenavi</span>
      </div>
      <nav className="flex-1 px-1 flex flex-col gap-2" style={{ overflowY: 'auto' }}>
        {menuItems.map(item => (
          <MenuItem
            key={item.name}
            icon={item.icon}
            label={item.name}
            active={location.pathname === item.path}
            onClick={() => {
              navigate(item.path);
              setMenuOpen(false);
            }}
          />
        ))}
      </nav>
    </aside>
  );
}
