import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faEnvelope, faLock, faUser } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGoogle as faGoogleBrand } from '@fortawesome/free-brands-svg-icons';
import './AuthModal.css';

export default function AuthModal({ open, onClose, twoFAState, onAuth, on2FA, onSignup, onCancel, onRequestNewCode }) {
  const [tab, setTab] = useState('login');
  const [loginForm, setLoginForm] = useState({ email: '', password: '', twofa: '' });
  const [signupForm, setSignupForm] = useState({ userName: '', email: '', password: '' });
  const [loginError, setLoginError] = useState('');
  const [signupError, setSignupError] = useState('');
  const modalRef = useRef();

  useEffect(() => {
    if (!open) return;
    function handle(e) {
      if (modalRef.current && !modalRef.current.contains(e.target)) onClose();
    }
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [open, onClose]);

  useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  const contentClass = `auth-modal-content-inner${tab === 'login' ? ' login' : ' signup'}`;

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoginError('');
    const success = await onAuth(loginForm.email, loginForm.password);
    if (success) {
      onClose();
    }
  };

  const handle2FA = async (e) => {
    e.preventDefault();
    setLoginError('');
    if (!twoFAState) return;
    const success = await on2FA(loginForm.twofa);
    if (success) {
      onClose();
    }
  };

  const handleSignup = async (e) => {
    e.preventDefault();
    setSignupError('');
    if (!signupForm.userName || !signupForm.email || !signupForm.password) {
      setSignupError('Please fill all fields');
      return;
    }
    const success = await onSignup(signupForm.userName, signupForm.email, signupForm.password);
    if (success) {
      // Optionally auto-login after signup or switch to login tab
      setTab('login');
    } else {
      setSignupError('Signup failed. Please try again.');
    }
  };

  return (
    <div className={`auth-modal-backdrop${open ? ' open' : ''}`}>
      <div className={`auth-modal-glass${open ? ' open' : ''}`} ref={modalRef}>
        <div className="auth-modal-tabs">
          <button className={tab === 'login' ? 'active' : ''} onClick={() => setTab('login')} type="button">Log In</button>
          <button className={tab === 'signup' ? 'active' : ''} onClick={() => setTab('signup')} type="button">Sign Up</button>
        </div>
        <div className="auth-modal-content-outer">
          <div className={contentClass}>
            {/* LOGIN FORM */}
            {!twoFAState ? (
              <form className="auth-modal-form" onSubmit={handleLogin} autoComplete="on">
                <div className="auth-modal-header">
                  <FontAwesomeIcon icon={faUserCircle} className="auth-modal-avatar" />
                  <span className="auth-modal-title">Welcome Back</span>
                </div>
                <div className="auth-modal-field">
                  <input
                    type="email"
                    className="auth-modal-input"
                    value={loginForm.email}
                    onChange={e => setLoginForm(f => ({ ...f, email: e.target.value }))}
                    autoComplete="username"
                    required
                    placeholder=" "
                    id="login-email"
                  />
                  <label className="auth-modal-label" htmlFor="login-email">Email</label>
                </div>
                <div className="auth-modal-field">
                  <input
                    type="password"
                    className="auth-modal-input"
                    value={loginForm.password}
                    onChange={e => setLoginForm(f => ({ ...f, password: e.target.value }))}
                    autoComplete="current-password"
                    required
                    placeholder=" "
                    id="login-password"
                  />
                  <label className="auth-modal-label" htmlFor="login-password">Password</label>
                </div>
                {loginError && <div className="auth-modal-error">{loginError}</div>}
                <button className="auth-modal-btn" type="submit">Log In</button>
                <button
                  className="auth-modal-btn secondary"
                  type="button"
                  onClick={() => setLoginForm({ email: 'test@test.com', password: 'test' })}
                >
                  Use test credentials
                </button>
                <div className="auth-modal-divider" />
                <div className="auth-modal-oauth-label">Continue with</div>
                <div className="auth-modal-oauth-btns">
                  <button className="auth-modal-oauth-btn google" type="button" disabled>
                    <span className="oauth-icon">
                      <FontAwesomeIcon icon={faGoogleBrand} />
                    </span>
                    Google
                  </button>
                </div>
              </form>
            ) : (
              <form className="auth-modal-form" onSubmit={handle2FA} autoComplete="off">
                <div className="auth-modal-header">
                  <FontAwesomeIcon icon={faLock} className="auth-modal-avatar" />
                  <span className="auth-modal-title">Enter 2FA Code</span>
                  <div className="auth-modal-subtitle">
                    {twoFAState?.type === 'email' ? 'Check your email for the verification code' : 'Enter your 2FA code'}
                  </div>
                </div>
                <div className="auth-modal-field">
                  <input
                    type="text"
                    className="auth-modal-input"
                    value={loginForm.twofa}
                    onChange={e => setLoginForm(f => ({ ...f, twofa: e.target.value }))}
                    required
                    placeholder=" "
                    id="login-2fa"
                  />
                  <label className="auth-modal-label" htmlFor="login-2fa">2FA Code</label>
                </div>
                {loginError && <div className="auth-modal-error">{loginError}</div>}
                <button className="auth-modal-btn" type="submit">Verify</button>
                {twoFAState?.type === 'email' && (
                  <button 
                    className="auth-modal-btn secondary" 
                    type="button" 
                    onClick={onRequestNewCode}
                  >
                    Request New Code
                  </button>
                )}
                <button 
                  className="auth-modal-btn secondary" 
                  type="button" 
                  onClick={onCancel}
                >
                  Cancel Login
                </button>
              </form>
            )}
            {/* SIGNUP FORM */}
            <form className="auth-modal-form" onSubmit={handleSignup} autoComplete="on">
              <div className="auth-modal-header">
                <FontAwesomeIcon icon={faUserCircle} className="auth-modal-avatar" />
                <span className="auth-modal-title">Sign Up</span>
              </div>
              <div className="auth-modal-field">
                <input
                  type="text"
                  className="auth-modal-input"
                  value={signupForm.userName}
                  onChange={e => setSignupForm(f => ({ ...f, userName: e.target.value }))}
                  autoComplete="username"
                  required
                  placeholder=" "
                  id="signup-username"
                />
                <label className="auth-modal-label" htmlFor="signup-username">Username</label>
              </div>
              <div className="auth-modal-field">
                <input
                  type="email"
                  className="auth-modal-input"
                  value={signupForm.email}
                  onChange={e => setSignupForm(f => ({ ...f, email: e.target.value }))}
                  autoComplete="email"
                  required
                  placeholder=" "
                  id="signup-email"
                />
                <label className="auth-modal-label" htmlFor="signup-email">Email</label>
              </div>
              <div className="auth-modal-field">
                <input
                  type="password"
                  className="auth-modal-input"
                  value={signupForm.password}
                  onChange={e => setSignupForm(f => ({ ...f, password: e.target.value }))}
                  autoComplete="new-password"
                  required
                  placeholder=" "
                  id="signup-password"
                />
                <label className="auth-modal-label" htmlFor="signup-password">Password</label>
              </div>
              {signupError && <div className="auth-modal-error">{signupError}</div>}
              <button className="auth-modal-btn" type="submit">Sign Up</button>
              <div className="auth-modal-divider" />
              <div className="auth-modal-oauth-label">Continue with</div>
              <div className="auth-modal-oauth-btns">
                <button className="auth-modal-oauth-btn google" type="button" disabled>
                  <span className="oauth-icon">
                    <FontAwesomeIcon icon={faGoogleBrand} />
                  </span>
                  Google
                </button>
              </div>
            </form>
          </div>
        </div>
        <button className="auth-modal-close" onClick={onClose} aria-label="Close">&times;</button>
      </div>
    </div>
  );
}
