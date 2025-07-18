import React, { useState, useRef, useEffect } from 'react';
import { faUserCircle, faEnvelope, faLock, faUser } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGoogle as faGoogleBrand } from '@fortawesome/free-brands-svg-icons';
import { requestPasswordReset, confirmPasswordReset } from '../../../services/authService';
import './AuthModal.css';

export default function AuthModal({ open, onClose, twoFAState, onAuth, on2FA, onSignup, onCancel, onRequestNewCode }) {
  const [tab, setTab] = useState('login');
  const [loginForm, setLoginForm] = useState({ email: '', password: '', twofa: '' });
  const [signupForm, setSignupForm] = useState({ firstName: '', lastName: '', userName: '', email: '', password: '' });
  const [loginError, setLoginError] = useState('');
  const [signupError, setSignupError] = useState('');
  const [showForgotPassword, setShowForgotPassword] = useState(false);
  const [forgotPasswordForm, setForgotPasswordForm] = useState({ email: '', code: '', newPassword: '', confirmPassword: '' });
  const [forgotPasswordStep, setForgotPasswordStep] = useState(1); // 1: email, 2: code + password
  const [forgotPasswordError, setForgotPasswordError] = useState('');
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

  const handleCancel = () => {
    // Reset form state
    setLoginForm({ email: '', password: '', twofa: '' });
    setLoginError('');
    setTab('login');
    // Call the parent cancel function
    onCancel();
  };

  const contentClass = `auth-modal-content-inner${tab === 'login' ? ' login' : ' signup'}`;

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoginError('');
    const result = await onAuth(loginForm.email, loginForm.password);
    if (result && result.success) {
      onClose();
    } else if (result && result.error) {
      setLoginError(result.error);
    } else if (result && !result.twoFA) {
      setLoginError('Login failed. Please check your credentials.');
    }
  };

  const handle2FA = async (e) => {
    e.preventDefault();
    setLoginError('');
    if (!twoFAState) return;
    const result = await on2FA(loginForm.twofa);
    if (result && result.success) {
      onClose();
    } else if (result && result.error) {
      // Make sure error is always a string
      const errorMessage = typeof result.error === 'string' ? result.error : 
                          (result.error?.message || result.error?.error || "2FA verification failed");
      setLoginError(errorMessage);
    } else {
      setLoginError("2FA verification failed");
    }
  };

  const handleSignup = async (e) => {
    e.preventDefault();
    if (!signupForm.firstName.trim() || !signupForm.lastName.trim() || !signupForm.userName.trim() || !signupForm.email.trim() || !signupForm.password.trim()) {
      setSignupError('All fields are required');
      return;
    }
    setSignupError('');
    const result = await onSignup(signupForm.firstName, signupForm.lastName, signupForm.userName, signupForm.email, signupForm.password);
    if (!result.success) {
      setSignupError(result.error || 'Signup failed');
    }
  };

  const handleForgotPassword = async (e) => {
    e.preventDefault();
    setForgotPasswordError('');
    
    if (forgotPasswordStep === 1) {
      // Request password reset code
      if (!forgotPasswordForm.email.trim()) {
        setForgotPasswordError('Email is required');
        return;
      }
      const resp = await requestPasswordReset(forgotPasswordForm.email);
      if (resp.success) {
        setForgotPasswordStep(2);
        setForgotPasswordError('');
      } else {
        setForgotPasswordError(resp.error || 'Failed to send reset code');
      }
    } else {
      // Confirm password reset
      if (!forgotPasswordForm.code.trim() || !forgotPasswordForm.newPassword.trim() || !forgotPasswordForm.confirmPassword.trim()) {
        setForgotPasswordError('All fields are required');
        return;
      }
      if (forgotPasswordForm.newPassword !== forgotPasswordForm.confirmPassword) {
        setForgotPasswordError('Passwords do not match');
        return;
      }
      const resp = await confirmPasswordReset(forgotPasswordForm.email, forgotPasswordForm.code, forgotPasswordForm.newPassword);
      if (resp.success) {
        setShowForgotPassword(false);
        setForgotPasswordStep(1);
        setForgotPasswordForm({ email: '', code: '', newPassword: '', confirmPassword: '' });
        setLoginError('Password reset successful! Please log in with your new password.');
      } else {
        setForgotPasswordError(resp.error || 'Password reset failed');
      }
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
                <div style={{ width: '100%', textAlign: 'right', marginBottom: '0.2rem' }}>
                  <button
                    type="button"
                    className="auth-modal-btn secondary"
                    style={{ width: 'auto', fontSize: '0.98rem', padding: '0.2em 0.7em', margin: 0, background: 'none', color: 'var(--color-primary)', fontWeight: 600 }}
                    tabIndex={0}
                    onClick={() => {
                      setShowForgotPassword(true);
                      setLoginError('');
                    }}
                  >
                    Forgot password?
                  </button>
                </div>
                <button className="auth-modal-btn" type="submit">Log In</button>
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
                  onClick={handleCancel}
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
              <div style={{ display: 'flex', gap: '1rem' }}>
                <div className="auth-modal-field" style={{ flex: 1 }}>
                  <input
                    type="text"
                    placeholder=" "
                    value={signupForm.firstName}
                    onChange={e => setSignupForm(prev => ({ ...prev, firstName: e.target.value }))}
                    className="auth-modal-input"
                    required
                    id="signup-firstname"
                  />
                  <label className="auth-modal-label" htmlFor="signup-firstname">First Name</label>
                </div>

                <div className="auth-modal-field" style={{ flex: 1 }}>
                  <input
                    type="text"
                    placeholder=" "
                    value={signupForm.lastName}
                    onChange={e => setSignupForm(prev => ({ ...prev, lastName: e.target.value }))}
                    className="auth-modal-input"
                    required
                    id="signup-lastname"
                  />
                  <label className="auth-modal-label" htmlFor="signup-lastname">Last Name</label>
                </div>
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
        
        {/* Forgot Password Overlay */}
        {showForgotPassword && (
          <div className="auth-modal-forgot-overlay">
            <div className="auth-modal-forgot-content">
              <form onSubmit={handleForgotPassword} autoComplete="on">
                <div className="auth-modal-header">
                  <FontAwesomeIcon icon={faLock} className="auth-modal-avatar" />
                  <span className="auth-modal-title">{forgotPasswordStep === 1 ? 'Forgot Password' : 'Reset Password'}</span>
                  <div className="auth-modal-subtitle">
                    {forgotPasswordStep === 1 ? 'Enter your email to receive a verification code' : 'Enter the code and your new password'}
                  </div>
                </div>
                {forgotPasswordStep === 1 ? (
                  <div className="auth-modal-field">
                    <input
                      type="email"
                      className="auth-modal-input"
                      value={forgotPasswordForm.email}
                      onChange={e => setForgotPasswordForm(f => ({ ...f, email: e.target.value }))}
                      autoComplete="email"
                      required
                      placeholder=" "
                      id="forgot-password-email"
                    />
                    <label className="auth-modal-label" htmlFor="forgot-password-email">Email</label>
                  </div>
                ) : (
                  <>
                    <div className="auth-modal-field">
                      <input
                        type="text"
                        className="auth-modal-input"
                        value={forgotPasswordForm.code}
                        onChange={e => setForgotPasswordForm(f => ({ ...f, code: e.target.value }))}
                        required
                        placeholder=" "
                        id="forgot-password-code"
                      />
                      <label className="auth-modal-label" htmlFor="forgot-password-code">Verification Code</label>
                    </div>
                    <div className="auth-modal-field">
                      <input
                        type="password"
                        className="auth-modal-input"
                        value={forgotPasswordForm.newPassword}
                        onChange={e => setForgotPasswordForm(f => ({ ...f, newPassword: e.target.value }))}
                        autoComplete="new-password"
                        required
                        placeholder=" "
                        id="forgot-password-new"
                      />
                      <label className="auth-modal-label" htmlFor="forgot-password-new">New Password</label>
                    </div>
                    <div className="auth-modal-field">
                      <input
                        type="password"
                        className="auth-modal-input"
                        value={forgotPasswordForm.confirmPassword}
                        onChange={e => setForgotPasswordForm(f => ({ ...f, confirmPassword: e.target.value }))}
                        autoComplete="new-password"
                        required
                        placeholder=" "
                        id="forgot-password-confirm"
                      />
                      <label className="auth-modal-label" htmlFor="forgot-password-confirm">Confirm Password</label>
                    </div>
                  </>
                )}
                {forgotPasswordError && <div className="auth-modal-error">{forgotPasswordError}</div>}
                <button className="auth-modal-btn" type="submit">
                  {forgotPasswordStep === 1 ? 'Send Code' : 'Reset Password'}
                </button>
                {forgotPasswordStep === 2 && (
                  <button 
                    className="auth-modal-btn secondary" 
                    type="button" 
                    onClick={() => setForgotPasswordStep(1)}
                  >
                    Back
                  </button>
                )}
                <button
                  className="auth-modal-btn secondary"
                  type="button"
                  onClick={() => {
                    setShowForgotPassword(false);
                    setForgotPasswordStep(1);
                    setForgotPasswordForm({ email: '', code: '', newPassword: '', confirmPassword: '' });
                    setForgotPasswordError('');
                  }}
                >
                  Cancel
                </button>
              </form>
            </div>
          </div>
        )}
        
        <button className="auth-modal-close" onClick={onClose} aria-label="Close">&times;</button>
      </div>
    </div>
  );
}
