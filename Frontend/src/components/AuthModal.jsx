import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faEnvelope, faLock } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGoogle as faGoogleBrand } from '@fortawesome/free-brands-svg-icons';
import './AuthModal.css';

export default function AuthModal({ open, onClose, login }) {
  const [tab, setTab] = useState('login');
  const [loginForm, setLoginForm] = useState({ email: '', password: '' });
  const [signupForm, setSignupForm] = useState({ email: '', password: '' });
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

  // Animate scroll between login/signup
  const contentClass = `auth-modal-content-inner${tab === 'login' ? ' login' : ' signup'}`;

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoginError('');
    const ok = await login(loginForm.email, loginForm.password);
    if (!ok) setLoginError('Invalid credentials');
    else onClose();
  };

  const handleSignup = async (e) => {
    e.preventDefault();
    setSignupError('');
    // Simulate signup (for demo)
    if (!signupForm.email || !signupForm.password) {
      setSignupError('Please fill all fields');
      return;
    }
    // Accept any signup for demo
    onClose();
  };

  return (
    <div className={`auth-modal-backdrop${open ? ' open' : ''}`}>
      <div className={`auth-modal-glass${open ? ' open' : ''}`} ref={modalRef}>
        <div className="auth-modal-tabs">
          <button
            className={tab === 'login' ? 'active' : ''}
            onClick={() => setTab('login')}
            type="button"
          >Log In</button>
          <button
            className={tab === 'signup' ? 'active' : ''}
            onClick={() => setTab('signup')}
            type="button"
          >Sign Up</button>
        </div>
        <div className="auth-modal-content-outer">
          <div className={contentClass}>
            {/* LOGIN FORM */}
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
              <div style={{ width: '100%', textAlign: 'right', marginBottom: '0.2rem' }}>
                <button
                  type="button"
                  className="auth-modal-btn secondary"
                  style={{ width: 'auto', fontSize: '0.98rem', padding: '0.2em 0.7em', margin: 0, background: 'none', color: 'var(--color-primary)', fontWeight: 600 }}
                  tabIndex={0}
                  onClick={() => { /* forgot password action */ }}
                >
                  Forgot password?
                </button>
              </div>
              {loginError && <div className="auth-modal-error">{loginError}</div>}
              <button className="auth-modal-btn" type="submit">
                Log In
              </button>
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
            {/* SIGNUP FORM */}
            <form className="auth-modal-form" onSubmit={handleSignup} autoComplete="on">
              <div className="auth-modal-header">
                <FontAwesomeIcon icon={faUserCircle} className="auth-modal-avatar" />
                <span className="auth-modal-title">Sign Up</span>
              </div>
              <div className="auth-modal-field">
                <input
                  type="email"
                  className="auth-modal-input"
                  value={signupForm.email}
                  onChange={e => setSignupForm(f => ({ ...f, email: e.target.value }))}
                  autoComplete="username"
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
              <button className="auth-modal-btn" type="submit">
                Sign Up
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
          </div>
        </div>
        <button className="auth-modal-close" onClick={onClose} aria-label="Close">&times;</button>
      </div>
    </div>
  );
}
