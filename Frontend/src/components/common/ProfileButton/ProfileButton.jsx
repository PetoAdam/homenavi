import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faSignOutAlt, faCog } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './ProfileButton.css';
import { loginTestUser } from '../../../services/authService';
import AuthModal from '../../Auth/AuthModal/AuthModal';

// Mock auth hook using test login
function useAuth() {
  const [user, setUser] = useState({
    isLoggedIn: false,
    name: '',
    avatar: '',
    email: '',
  });

  const login = async (email, password) => {
    const result = await loginTestUser(email, password);
    if (result.success) {
      setUser({
        isLoggedIn: true,
        name: result.name,
        avatar: result.avatar,
        email: result.email,
      });
      return true;
    }
    return false;
  };

  const logout = () => setUser({
    isLoggedIn: false,
    name: '',
    avatar: '',
    email: '',
  });

  return { user, login, logout };
}

export default function ProfileButton() {
  const { user, login, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [logoutMsg, setLogoutMsg] = useState("");
  const btnRef = useRef();

  // Close popover on outside click
  useEffect(() => {
    if (!open) return;
    function handle(e) {
      if (btnRef.current && !btnRef.current.contains(e.target)) setOpen(false);
    }
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [open]);

  // Show logout message for 2.5 seconds
  useEffect(() => {
    if (logoutMsg) {
      const timer = setTimeout(() => setLogoutMsg(""), 2500);
      return () => clearTimeout(timer);
    }
  }, [logoutMsg]);

  const handleLogout = () => {
    logout();
    setOpen(false);
    setLogoutMsg("Logged out successfully");
  };

  return (
    <div className="profile-btn-wrap" ref={btnRef}>
      {/* Toast notification for logout */}
      <div className={`profile-toast${logoutMsg ? ' profile-toast--show' : ''}`}>{logoutMsg}</div>
      {!user.isLoggedIn && (
        <button
          className="profile-login-text hide-on-mobile"
          onClick={() => setShowAuthModal(true)}
        >
          Log in
        </button>
      )}
      <button
        className="profile-avatar-btn"
        onClick={() => user.isLoggedIn ? setOpen(o => !o) : setShowAuthModal(true)}
        aria-label="Profile"
      >
        {user.isLoggedIn && user.avatar ? (
          <img src={user.avatar} alt="Profile" className="profile-avatar-img" />
        ) : (
          <FontAwesomeIcon icon={faUserCircle} className="profile-avatar-icon" />
        )}
      </button>
      {open && user.isLoggedIn && (
        <div className="profile-popover profile-popover-solid">
          <div className="profile-menu">
            <div className="profile-menu-header">
              <img src={user.avatar} alt="Profile" className="profile-menu-avatar" />
              <span className="profile-menu-name">{user.name}</span>
            </div>
            <button className="profile-menu-item" onClick={() => { /* settings */ }}>
              <FontAwesomeIcon icon={faCog} className="profile-menu-icon" /> Settings
            </button>
            <button className="profile-menu-item" onClick={handleLogout}>
              <FontAwesomeIcon icon={faSignOutAlt} className="profile-menu-icon" /> Logout
            </button>
          </div>
        </div>
      )}
      <AuthModal
        open={showAuthModal}
        onClose={() => setShowAuthModal(false)}
        login={login}
      />
    </div>
  );
}
