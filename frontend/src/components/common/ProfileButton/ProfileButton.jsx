import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faSignOutAlt, faCog } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './ProfileButton.css';
import AuthModal from '../../Auth/AuthModal/AuthModal';
import UserSettings from '../../UserSettings/UserSettings';
import { useAuth } from '../../../context/AuthContext';

export default function ProfileButton() {
  const { user, handleLogin, handleLogout, handle2FA, handleSignup, cancelLogin: authCancelLogin, requestNew2FACode } = useAuth();
  const [twoFAState, setTwoFAState] = useState(null);
  const [open, setOpen] = useState(false);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [toastMsg, setToastMsg] = useState("");
  const btnRef = useRef();

  useEffect(() => {
    if (!open) return;
    function handle(e) {
      if (btnRef.current && !btnRef.current.contains(e.target)) setOpen(false);
    }
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [open]);

  useEffect(() => {
    if (toastMsg) {
      const timer = setTimeout(() => setToastMsg(""), 2500);
      return () => clearTimeout(timer);
    }
  }, [toastMsg]);

  useEffect(() => {
    if (user && showAuthModal) setShowAuthModal(false);
  }, [user, showAuthModal]);

  const doLogout = async () => {
    await handleLogout();
    setOpen(false);
    setToastMsg("Logged out successfully");
  };

  const doLogin = async (email, password) => {
    const resp = await handleLogin(email, password);
    if (resp.twoFA) {
      setTwoFAState({ userId: resp.userId, type: resp.type });
      return false;
    }
    if (resp.success) {
      setToastMsg("Logged in successfully");
      setTwoFAState(null);
      return true;
    }
    return false;
  };

  const doSignup = async (userName, email, password) => {
    const resp = await handleSignup(userName, email, password);
    if (resp.success) {
      setToastMsg("Account created successfully! Please log in.");
      return true;
    }
    return false;
  };

  const do2FA = async (code) => {
    if (!twoFAState) return false;
    const resp = await handle2FA(code);
    if (resp.success) {
      setToastMsg("Logged in successfully");
      setTwoFAState(null);
      return true;
    }
    setToastMsg(resp.error || "2FA failed");
    return false;
  };

  const requestNewCode = async () => {
    const resp = await requestNew2FACode();
    if (resp.success) {
      setToastMsg("New code sent to your email");
    } else {
      setToastMsg(resp.error || "Failed to send new code");
    }
  };

  const cancelLogin = () => {
    authCancelLogin();
    setTwoFAState(null);
    setShowAuthModal(false);
    setToastMsg("Login cancelled");
  };

  return (
    <div className="profile-btn-wrap" ref={btnRef}>
      <div className={`profile-toast${toastMsg ? ' profile-toast--show' : ''}`}>{toastMsg}</div>
      {!user && (
        <button className="profile-login-text hide-on-mobile" onClick={() => setShowAuthModal(true)}>
          Log in
        </button>
      )}
      <button
        className="profile-avatar-btn"
        onClick={() => user ? setOpen(o => !o) : setShowAuthModal(true)}
        aria-label="Profile"
      >
        {user && user.avatar ? (
          <img src={user.avatar} alt="Profile" className="profile-avatar-img" />
        ) : (
          <FontAwesomeIcon icon={faUserCircle} className="profile-avatar-icon" />
        )}
      </button>
      {open && user && (
        <div className="profile-popover profile-popover-solid">
          <div className="profile-menu">
            <div className="profile-menu-header">
              {user.avatar && <img src={user.avatar} alt="Profile" className="profile-menu-avatar" />}
              <span className="profile-menu-name">{user.user_name || user.name}</span>
            </div>
            <button className="profile-menu-item" onClick={() => { setShowSettings(true); setOpen(false); }}>
              <FontAwesomeIcon icon={faCog} className="profile-menu-icon" /> Settings
            </button>
            <button className="profile-menu-item" onClick={doLogout}>
              <FontAwesomeIcon icon={faSignOutAlt} className="profile-menu-icon" /> Logout
            </button>
          </div>
        </div>
      )}
      <AuthModal
        open={showAuthModal}
        onClose={() => setShowAuthModal(false)}
        twoFAState={twoFAState}
        onAuth={doLogin}
        on2FA={do2FA}
        onSignup={doSignup}
        onCancel={cancelLogin}
        onRequestNewCode={requestNewCode}
      />
      {showSettings && (
        <UserSettings onClose={() => setShowSettings(false)} />
      )}
    </div>
  );
}
