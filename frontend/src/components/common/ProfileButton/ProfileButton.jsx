import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faSignOutAlt, faCog } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './ProfileButton.css';
import AuthModal from '../../Auth/AuthModal/AuthModal';
import UserSettings from '../../UserSettings/UserSettings';
import { useAuth } from '../../../context/AuthContext';
import UserAvatar from '../../common/UserAvatar/UserAvatar';

export default function ProfileButton() {
  const { user, handleLogin, handleLogout, handle2FA, handleSignup, cancelLogin: authCancelLogin, requestNew2FACode, loading } = useAuth();
  const [twoFAState, setTwoFAState] = useState(null);
  const [open, setOpen] = useState(false);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [toastMsg, setToastMsg] = useState("");
  const [lastRequestTime, setLastRequestTime] = useState(0);
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

  useEffect(() => {
    const handler = (e) => {
      if (user) return;
      setShowAuthModal(true);
    };
    window.addEventListener('homenavi:open-auth', handler);
    return () => window.removeEventListener('homenavi:open-auth', handler);
  }, [user]);

  const doLogout = async () => {
    await handleLogout();
    setOpen(false);
    setToastMsg("Logged out successfully");
  };

  const doLogin = async (email, password) => {
    const resp = await handleLogin(email, password);
    if (resp.twoFA) {
      setTwoFAState({ userId: resp.userId, type: resp.type });
      return resp; // contains twoFA flag
    }
    if (resp.success) {
      setToastMsg("Logged in successfully");
      setTwoFAState(null);
      return { success: true };
    }
    // Pass through full failure object (lockout fields etc.)
    return resp;
  };

  const doSignup = async (firstName, lastName, userName, email, password) => {
    setTwoFAState(null);
    const result = await handleSignup(firstName, lastName, userName, email, password);
    if (result.success) {
      setShowAuthModal(false);
      setToastMsg("Account created successfully!");
    }
    return result;
  };

  const do2FA = async (code) => {
    if (!twoFAState) return { success: false, error: "No 2FA state" };
    const resp = await handle2FA(code);
    if (resp.success) {
      setToastMsg("Logged in successfully");
      setTwoFAState(null);
      return { success: true };
    }
    // Make sure error is always a string
    const errorMessage = typeof resp.error === 'string' ? resp.error : 
                        (resp.error?.message || resp.error?.error || "2FA verification failed");
    setToastMsg(errorMessage);
    return { success: false, error: errorMessage };
  };

  const requestNewCode = async () => {
    const now = Date.now();
    const cooldownMs = 30000; // 30 seconds cooldown
    
    // Check if we're in cooldown period
    if (now - lastRequestTime < cooldownMs) {
      const remainingSeconds = Math.ceil((cooldownMs - (now - lastRequestTime)) / 1000);
      setToastMsg(`Please wait ${remainingSeconds}s before requesting another code`);
      return;
    }
    
    // Check if already loading to prevent double-clicking
    if (loading) {
      setToastMsg("Request already in progress...");
      return;
    }
    
    setLastRequestTime(now);
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
    // Don't close the modal - just reset to login state
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
        <UserAvatar user={user} size={36} />
      </button>
      {open && user && (
        <div className="profile-popover profile-popover-solid">
          <div className="profile-menu">
            <div className="profile-menu-header">
              <UserAvatar user={user} size={36} />
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
        loading={loading}
      />
      {showSettings && (
        <UserSettings onClose={() => setShowSettings(false)} />
      )}
    </div>
  );
}
