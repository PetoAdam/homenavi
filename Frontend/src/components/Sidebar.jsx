import React, { forwardRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faHouse, faLightbulb, faMap } from '@fortawesome/free-solid-svg-icons';
import MenuItem from './MenuItem';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import './Sidebar.css';

const menuItems = [
  { name: 'Home', path: '/', icon: <FontAwesomeIcon icon={faHouse} /> },
  { name: 'Devices', path: '/devices', icon: <FontAwesomeIcon icon={faLightbulb} /> },
  { name: 'Map', path: '/map', icon: <FontAwesomeIcon icon={faMap} /> },
  { name: 'Spotify', path: '/spotify', icon: <FontAwesomeIcon icon={faSpotify} /> },
];

const Sidebar = forwardRef(function Sidebar({ menuOpen, setMenuOpen, isPermanentSidebar }, ref) {
  const location = useLocation();
  const navigate = useNavigate();

  return (
    <aside
      key={isPermanentSidebar ? 'permanent' : 'overlay'}
      className={`sidebar${menuOpen ? ' open' : ''} flex flex-col justify-start`}
      aria-hidden={!menuOpen && !isPermanentSidebar}
      tabIndex={-1}
      ref={ref}
    >
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
});

export default Sidebar;
