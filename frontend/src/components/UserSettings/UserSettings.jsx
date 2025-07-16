import React, { useState, useEffect } from 'react';
import { useAuth } from '../../context/AuthContext';
import { 
  getMe, 
  requestEmailVerify, 
  confirmEmailVerify,
  request2FAEmail,
  verify2FAEmail 
} from '../../services/authService';
import './UserSettings.css';

export default function UserSettings({ onClose }) {
  const { user, accessToken, handleLogout, refreshUser } = useAuth();
  const [emailVerified, setEmailVerified] = useState(user?.email_confirmed || false);
  const [twoFAEnabled, setTwoFAEnabled] = useState(user?.two_factor_enabled || false);
  const [status, setStatus] = useState('');
  const [emailCode, setEmailCode] = useState('');
  const [twoFACode, setTwoFACode] = useState('');
  const [showEmailCodeInput, setShowEmailCodeInput] = useState(false);
  const [show2FACodeInput, setShow2FACodeInput] = useState(false);

  // Sync local state with user data from AuthContext
  useEffect(() => {
    if (user) {
      setEmailVerified(user.email_confirmed || false);
      setTwoFAEnabled(user.two_factor_enabled || false);
    }
  }, [user]);

  const handleLogoutAndClose = async () => {
    await handleLogout();
    onClose();
  };

  const handleEmailVerify = async () => {
    if (!user?.id) return;
    setStatus('Requesting email verification...');
    const resp = await requestEmailVerify(user.id, accessToken);
    if (resp.success) {
      setStatus('Check your email for the code.');
      setShowEmailCodeInput(true);
    } else {
      setStatus(resp.error || 'Failed to request email verification');
    }
  };

  const handleEmailConfirm = async () => {
    if (!user?.id || !emailCode.trim()) return;
    setStatus('Confirming email...');
    const resp = await confirmEmailVerify(user.id, emailCode.trim(), accessToken);
    if (resp.success) {
      setStatus('✅ Email verified successfully!');
      setEmailCode('');
      setShowEmailCodeInput(false);
      // Refresh user data from backend to get updated email_confirmed status
      await refreshUser();
    } else {
      setStatus('❌ ' + (resp.error || 'Email verification failed'));
    }
  };

  const handle2FASetup = async () => {
    if (!user?.id) return;
    setStatus('Setting up 2FA...');
    const resp = await request2FAEmail(user.id, accessToken);
    if (resp.success) {
      setStatus('Check your email for the 2FA code.');
      setShow2FACodeInput(true);
    } else {
      setStatus('❌ ' + (resp.error || 'Failed to setup 2FA'));
    }
  };

  const handle2FAVerify = async () => {
    if (!user?.id || !twoFACode.trim()) return;
    setStatus('Verifying 2FA...');
    const resp = await verify2FAEmail(user.id, twoFACode.trim(), accessToken);
    if (resp.success) {
      setStatus('✅ 2FA enabled successfully!');
      setTwoFACode('');
      setShow2FACodeInput(false);
      // Refresh user data from backend to get updated two_factor_enabled status
      await refreshUser();
    } else {
      setStatus('❌ ' + (resp.error || '2FA verification failed'));
    }
  };

  return (
    <div className="user-settings">
      <div className="user-settings-header">
        <h2>Profile Settings</h2>
        <button onClick={onClose} className="close-btn">
          ×
        </button>
      </div>
      
      <div className="user-settings-section">
        <div className="user-settings-field">
          <strong>Email:</strong> {user?.email}
        </div>
        <div className="user-settings-field">
          <strong>Username:</strong> {user?.user_name}
        </div>
        <div className="user-settings-field">
          <strong>Email Verified:</strong> 
          <span style={{ 
            color: emailVerified ? 'var(--color-primary)' : 'var(--color-secondary-light)',
            marginLeft: '0.5rem',
            fontWeight: '600'
          }}>
            {emailVerified ? '✅ Verified' : '❌ Not Verified'}
          </span>
        </div>
      </div>

      <div className="user-settings-section">
        <h3 style={{ color: 'var(--color-white)', fontSize: '1.1rem', marginBottom: '1rem', fontWeight: '600' }}>
          Email Verification
        </h3>
        <button onClick={handleEmailVerify} disabled={emailVerified || showEmailCodeInput}>
          {emailVerified ? 'Email Already Verified' : showEmailCodeInput ? 'Code Sent' : 'Send Verification Email'}
        </button>
        
        {showEmailCodeInput && !emailVerified && (
          <div className="user-settings-input-group">
            <input 
              type="text" 
              placeholder="Enter email verification code" 
              value={emailCode}
              onChange={e => setEmailCode(e.target.value)}
            />
            <button onClick={handleEmailConfirm} disabled={!emailCode.trim()}>
              Verify
            </button>
          </div>
        )}
      </div>

      <div className="user-settings-section">
        <h3 style={{ color: 'var(--color-white)', fontSize: '1.1rem', marginBottom: '1rem', fontWeight: '600' }}>
          Two-Factor Authentication
        </h3>
        <div className="user-settings-field">
          <strong>2FA Status:</strong> 
          <span style={{ 
            color: twoFAEnabled ? 'var(--color-primary)' : 'var(--color-secondary-light)',
            marginLeft: '0.5rem',
            fontWeight: '600'
          }}>
            {twoFAEnabled ? '✅ Enabled' : '❌ Disabled'}
          </span>
        </div>
        <button onClick={handle2FASetup} disabled={twoFAEnabled || show2FACodeInput}>
          {twoFAEnabled ? '2FA Already Enabled' : show2FACodeInput ? 'Code Sent' : 'Enable Email 2FA'}
        </button>
        
        {show2FACodeInput && !twoFAEnabled && (
          <div className="user-settings-input-group">
            <input 
              type="text" 
              placeholder="Enter 2FA verification code" 
              value={twoFACode}
              onChange={e => setTwoFACode(e.target.value)}
            />
            <button onClick={handle2FAVerify} disabled={!twoFACode.trim()}>
              Enable 2FA
            </button>
          </div>
        )}
      </div>

      <div className="user-settings-section">
        <button onClick={handleLogoutAndClose} className="secondary">
          Logout
        </button>
      </div>
      
      {status && (
        <div className={`user-settings-status ${
          status.includes('✅') ? 'success' : 
          status.includes('❌') ? 'error' : ''
        }`}>
          {status}
        </div>
      )}
    </div>
  );
}
