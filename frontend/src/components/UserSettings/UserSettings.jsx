import React, { useState, useEffect } from 'react';
import { useAuth } from '../../context/AuthContext';
import { 
  getMe, 
  requestEmailVerify, 
  confirmEmailVerify,
  request2FAEmail,
  verify2FAEmail,
  changePassword,
  patchUser as patchUserService
} from '../../services/authService';
import './UserSettings.css';
import axios from 'axios';

export default function UserSettings({ onClose }) {
  const { user, accessToken, handleLogout, refreshUser } = useAuth();
  const [emailVerified, setEmailVerified] = useState(user?.email_confirmed || false);
  const [twoFAEnabled, setTwoFAEnabled] = useState(user?.two_factor_enabled || false);
  const [status, setStatus] = useState('');
  const [emailCode, setEmailCode] = useState('');
  const [twoFACode, setTwoFACode] = useState('');
  const [showEmailCodeInput, setShowEmailCodeInput] = useState(false);
  const [show2FACodeInput, setShow2FACodeInput] = useState(false);
  
  // Profile editing states
  const [editingProfile, setEditingProfile] = useState(false);
  const [profileForm, setProfileForm] = useState({
    firstName: user?.first_name || '',
    lastName: user?.last_name || ''
  });
  
  // Password reset states
  const [showPasswordReset, setShowPasswordReset] = useState(false);
  const [passwordForm, setPasswordForm] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: ''
  });

  // Sync local state with user data from AuthContext
  useEffect(() => {
    if (user) {
      setEmailVerified(user.email_confirmed || false);
      setTwoFAEnabled(user.two_factor_enabled || false);
      setProfileForm({
        firstName: user.first_name || '',
        lastName: user.last_name || ''
      });
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

  const handleProfileSave = async () => {
    if (!user?.id || !profileForm.firstName.trim() || !profileForm.lastName.trim()) {
      setStatus('❌ First name and last name are required');
      return;
    }
    setStatus('Updating profile...');
    const resp = await patchUserService(user.id, {
      first_name: profileForm.firstName.trim(),
      last_name: profileForm.lastName.trim()
    }, accessToken);
    if (resp.success) {
      setStatus('✅ Profile updated successfully!');
      setEditingProfile(false);
      await refreshUser();
    } else {
      setStatus('❌ ' + (resp.error || 'Profile update failed'));
    }
  };

  const handlePasswordReset = async () => {
    if (!passwordForm.currentPassword || !passwordForm.newPassword || !passwordForm.confirmPassword) {
      setStatus('❌ All password fields are required');
      return;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setStatus('❌ New passwords do not match');
      return;
    }
    if (passwordForm.newPassword.length < 8) {
      setStatus('❌ New password must be at least 8 characters');
      return;
    }
    setStatus('Updating password...');
    console.log('Attempting password change...');
    const resp = await changePassword(passwordForm.currentPassword, passwordForm.newPassword, accessToken);
    console.log('Password change response:', resp);
    if (resp.success) {
      setStatus('✅ Password updated successfully!');
      setPasswordForm({ currentPassword: '', newPassword: '', confirmPassword: '' });
      setShowPasswordReset(false);
    } else {
      setStatus('❌ ' + (resp.error || 'Password update failed'));
    }
  };

  return (
    <div className="user-settings-backdrop" onClick={onClose}>
      <div className="user-settings" onClick={e => e.stopPropagation()}>
        <div className="user-settings-header">
          <h2>Account Settings</h2>
        </div>
        
        <div className="user-settings-content-outer">
          <div className="user-settings-content">
          {/* Profile Card */}
          <div className="user-settings-card">
            <h3>
              <span className="user-settings-card-icon">👤</span>
              Profile
            </h3>
            
            <div className="profile-header">
              <div className="profile-avatar">
                {user?.first_name?.[0]?.toUpperCase()}{user?.last_name?.[0]?.toUpperCase()}
              </div>
              <div className="profile-info">
                <div className="profile-name">
                  {user?.first_name} {user?.last_name}
                </div>
                <div className="profile-email">
                  {user?.email}
                </div>
              </div>
            </div>
            
            <div className="user-settings-field">
              <strong>Username:</strong>
              <span style={{ color: 'var(--color-secondary-light)' }}>{user?.user_name}</span>
            </div>
            
            {editingProfile ? (
              <div className="edit-profile-form">
                <div className="name-inputs">
                  <input
                    type="text"
                    placeholder="First Name"
                    value={profileForm.firstName}
                    onChange={e => setProfileForm(prev => ({ ...prev, firstName: e.target.value }))}
                  />
                  <input
                    type="text"
                    placeholder="Last Name"
                    value={profileForm.lastName}
                    onChange={e => setProfileForm(prev => ({ ...prev, lastName: e.target.value }))}
                  />
                </div>
                <div className="button-group">
                  <button onClick={handleProfileSave}>Save Changes</button>
                  <button onClick={() => {
                    setEditingProfile(false);
                    setProfileForm({ firstName: user?.first_name || '', lastName: user?.last_name || '' });
                  }} className="secondary">Cancel</button>
                </div>
              </div>
            ) : (
              <button onClick={() => setEditingProfile(true)} className="secondary">
                Edit Profile
              </button>
            )}
            
            {status && status.includes('Profile') && (
              <div className={`card-status ${
                status.includes('✅') ? 'success' : 
                status.includes('❌') ? 'error' : ''
              }`}>
                {status}
              </div>
            )}
          </div>

          {/* Security Card */}
          <div className="user-settings-card">
            <h3>
              <span className="user-settings-card-icon">🔒</span>
              Security
            </h3>
            
            {/* Email Verification */}
            <div className="user-settings-field">
              <strong>Email Status:</strong>
              <span className={`status-indicator ${emailVerified ? 'verified' : 'unverified'}`}>
                {emailVerified ? '✅ Verified' : '❌ Not Verified'}
              </span>
            </div>
            
            <button onClick={handleEmailVerify} disabled={emailVerified || showEmailCodeInput}>
              {emailVerified ? 'Email Verified' : showEmailCodeInput ? 'Code Sent' : 'Verify Email'}
            </button>
            
            {showEmailCodeInput && !emailVerified && (
              <div className="user-settings-input-group">
                <input 
                  type="text" 
                  placeholder="Enter verification code" 
                  value={emailCode}
                  onChange={e => setEmailCode(e.target.value)}
                />
                <button onClick={handleEmailConfirm} disabled={!emailCode.trim()}>
                  Confirm Email
                </button>
              </div>
            )}
            
            {status && (status.includes('Email') || status.includes('email')) && (
              <div className={`card-status ${
                status.includes('✅') ? 'success' : 
                status.includes('❌') ? 'error' : ''
              }`}>
                {status}
              </div>
            )}
            
            {/* 2FA Section */}
            <div className="user-settings-field" style={{ marginTop: '1.5rem' }}>
              <strong>Two-Factor Authentication:</strong>
              <span className={`status-indicator ${twoFAEnabled ? 'verified' : 'unverified'}`}>
                {twoFAEnabled ? '✅ Enabled' : '❌ Disabled'}
              </span>
            </div>
            
            <button onClick={handle2FASetup} disabled={twoFAEnabled || show2FACodeInput}>
              {twoFAEnabled ? '2FA Enabled' : show2FACodeInput ? 'Code Sent' : 'Enable 2FA'}
            </button>
            
            {show2FACodeInput && !twoFAEnabled && (
              <div className="user-settings-input-group">
                <input 
                  type="text" 
                  placeholder="Enter 2FA code" 
                  value={twoFACode}
                  onChange={e => setTwoFACode(e.target.value)}
                />
                <button onClick={handle2FAVerify} disabled={!twoFACode.trim()}>
                  Enable 2FA
                </button>
              </div>
            )}
            
            {status && status.includes('2FA') && (
              <div className={`card-status ${
                status.includes('✅') ? 'success' : 
                status.includes('❌') ? 'error' : ''
              }`}>
                {status}
              </div>
            )}
            
            {/* Change Password */}
            <div style={{ marginTop: '1.5rem', paddingTop: '1.5rem', borderTop: '1px solid rgba(60,70,90,0.3)' }}>
              <button onClick={() => setShowPasswordReset(!showPasswordReset)} className="secondary">
                {showPasswordReset ? 'Cancel Password Change' : 'Change Password'}
              </button>
              
              {showPasswordReset && (
                <div className="user-settings-input-group">
                  <input
                    type="password"
                    placeholder="Current Password"
                    value={passwordForm.currentPassword}
                    onChange={e => setPasswordForm(prev => ({ ...prev, currentPassword: e.target.value }))}
                  />
                  <input
                    type="password"
                    placeholder="New Password"
                    value={passwordForm.newPassword}
                    onChange={e => setPasswordForm(prev => ({ ...prev, newPassword: e.target.value }))}
                  />
                  <input
                    type="password"
                    placeholder="Confirm New Password"
                    value={passwordForm.confirmPassword}
                    onChange={e => setPasswordForm(prev => ({ ...prev, confirmPassword: e.target.value }))}
                  />
                  <button onClick={handlePasswordReset} disabled={!passwordForm.currentPassword || !passwordForm.newPassword || !passwordForm.confirmPassword}>
                    Update Password
                  </button>
                </div>
              )}
              
              {status && (status.includes('Password') || status.includes('password')) && (
                <div className={`card-status ${
                  status.includes('✅') ? 'success' : 
                  status.includes('❌') ? 'error' : ''
                }`}>
                  {status}
                </div>
              )}
            </div>
          </div>

          {/* Logout Card */}
          <div className="user-settings-card">
            <h3>
              <span className="user-settings-card-icon">🚪</span>
              Account Actions
            </h3>
            
            <button onClick={handleLogoutAndClose} className="danger">
              Logout
            </button>
          </div>
        </div>
        </div>
        
        <button className="user-settings-close" onClick={onClose} aria-label="Close">&times;</button>
      </div>
    </div>
  );
}

// Patch user function (moved here from authService for clarity)
async function patchUser(userId, patch, accessToken) {
  try {
    const resp = await axios.patch(`/api/users/${userId}`, patch, {
      headers: {
        'Authorization': `Bearer ${accessToken}`,
        'Content-Type': 'application/json'
      }
    });
    return { success: true, data: resp.data };
  } catch (err) {
    return { success: false, error: err.response?.data || 'Update failed' };
  }
}
