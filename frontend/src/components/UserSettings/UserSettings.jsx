import React, { useEffect, useReducer } from 'react';
import { useAuth } from '../../context/AuthContext';
import { 
  requestEmailVerify, 
  confirmEmailVerify,
  request2FAEmail,
  verify2FAEmail,
  changePassword,
  patchUser as patchUserService,
  generateAvatar,
  uploadProfilePicture
} from '../../services/authService';
import './UserSettings.css';
import UserAvatar from '../common/UserAvatar/UserAvatar';
import BaseModal from '../common/BaseModal/BaseModal';
import { userSettingsInitialState, userSettingsReducer } from './userSettingsReducer';

export default function UserSettings({ onClose }) {
  const { user, accessToken, handleLogout, refreshUser } = useAuth();
  const [state, dispatch] = useReducer(userSettingsReducer, user, userSettingsInitialState);
  const {
    emailVerified,
    twoFAEnabled,
    status,
    emailCode,
    twoFACode,
    showEmailCodeInput,
    show2FACodeInput,
    editingProfile,
    profileForm,
    showPasswordReset,
    passwordForm,
    showProfilePictureModal,
    profilePictureFile,
  } = state;

  const setStateField = (key, value) => dispatch({ type: 'set-field', key, value });

  // Sync local state with user data from AuthContext
  useEffect(() => {
    if (user) {
      dispatch({ type: 'reset-from-user', user });
    }
  }, [user]);

  const handleLogoutAndClose = async () => {
    await handleLogout();
    onClose();
  };

  const handleEmailVerify = async () => {
    if (!user?.id) return;
    setStateField('status', 'Requesting email verification...');
    const resp = await requestEmailVerify(user.id, accessToken);
    if (resp.success) {
      setStateField('status', 'Check your email for the code.');
      setStateField('showEmailCodeInput', true);
    } else {
      setStateField('status', resp.error || 'Failed to request email verification');
    }
  };

  const handleEmailConfirm = async () => {
    if (!user?.id || !emailCode.trim()) return;
    setStateField('status', 'Confirming email...');
    const resp = await confirmEmailVerify(user.id, emailCode.trim(), accessToken);
    if (resp.success) {
      setStateField('status', '✅ Email verified successfully!');
      setStateField('emailCode', '');
      setStateField('showEmailCodeInput', false);
      // Refresh user data from backend to get updated email_confirmed status
      await refreshUser();
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Email verification failed'));
    }
  };

  const handle2FASetup = async () => {
    if (!user?.id) return;
    setStateField('status', 'Setting up 2FA...');
    const resp = await request2FAEmail(user.id, accessToken);
    if (resp.success) {
      setStateField('status', 'Check your email for the 2FA code.');
      setStateField('show2FACodeInput', true);
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Failed to setup 2FA'));
    }
  };

  const handle2FAVerify = async () => {
    if (!user?.id || !twoFACode.trim()) return;
    setStateField('status', 'Verifying 2FA...');
    const resp = await verify2FAEmail(user.id, twoFACode.trim(), accessToken);
    if (resp.success) {
      setStateField('status', '✅ 2FA enabled successfully!');
      setStateField('twoFACode', '');
      setStateField('show2FACodeInput', false);
      // Refresh user data from backend to get updated two_factor_enabled status
      await refreshUser();
    } else {
      setStateField('status', '❌ ' + (resp.error || '2FA verification failed'));
    }
  };

  const handleProfileSave = async () => {
    if (!user?.id || !profileForm.firstName.trim() || !profileForm.lastName.trim()) {
      setStateField('status', '❌ First name and last name are required');
      return;
    }
    setStateField('status', 'Updating profile...');
    const resp = await patchUserService(user.id, {
      first_name: profileForm.firstName.trim(),
      last_name: profileForm.lastName.trim()
    }, accessToken);
    if (resp.success) {
      setStateField('status', '✅ Profile updated successfully!');
      setStateField('editingProfile', false);
      await refreshUser();
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Profile update failed'));
    }
  };

  const handlePasswordReset = async () => {
    if (!passwordForm.currentPassword || !passwordForm.newPassword || !passwordForm.confirmPassword) {
      setStateField('status', '❌ All password fields are required');
      return;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setStateField('status', '❌ New passwords do not match');
      return;
    }
    if (passwordForm.currentPassword === passwordForm.newPassword) {
      setStateField('status', '❌ New password must be different from current password');
      return;
    }
    if (passwordForm.newPassword.length < 8) {
      setStateField('status', '❌ New password must be at least 8 characters');
      return;
    }
    setStateField('status', 'Updating password...');
    console.log('Attempting password change...');
    const resp = await changePassword(passwordForm.currentPassword, passwordForm.newPassword, accessToken);
    console.log('Password change response:', resp);
    if (resp.success) {
      setStateField('status', '✅ Password updated successfully!');
      dispatch({ type: 'reset-password-form' });
      setStateField('showPasswordReset', false);
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Password update failed'));
    }
  };

  const handleGenerateAvatar = async () => {
    setStateField('status', 'Generating avatar...');
    const resp = await generateAvatar(accessToken);
    if (resp.success) {
      setStateField('status', '✅ Avatar generated successfully!');
      await refreshUser();
      setStateField('showProfilePictureModal', false);
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Avatar generation failed'));
    }
  };

  const handleFileUpload = async () => {
    if (!profilePictureFile) {
      setStateField('status', '❌ Please select a file first');
      return;
    }
    setStateField('status', 'Uploading profile picture...');
    const resp = await uploadProfilePicture(profilePictureFile, accessToken);
    if (resp.success) {
      setStateField('status', '✅ Profile picture updated successfully!');
      await refreshUser();
      setStateField('showProfilePictureModal', false);
      setStateField('profilePictureFile', null);
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Upload failed'));
    }
  };

  const handleFileSelect = (event) => {
    const file = event.target.files[0];
    if (file) {
      // Validate file type
      if (!file.type.startsWith('image/')) {
        setStateField('status', '❌ Please select an image file');
        return;
      }
      // Validate file size (5MB max)
      if (file.size > 5 * 1024 * 1024) {
        setStateField('status', '❌ File too large (max 5MB)');
        return;
      }
      setStateField('profilePictureFile', file);
      setStateField('status', '');
    }
  };

  return (
    <BaseModal
      open
      onClose={onClose}
      dialogClassName="user-settings"
      closeAriaLabel="Close account settings"
    >
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
              <div 
                className="profile-avatar-container"
                onMouseEnter={() => {}}
                onMouseLeave={() => {}}
                onClick={() => setStateField('showProfilePictureModal', true)}
              >
                <div className="profile-avatar">
                  <UserAvatar user={user} size={64} />
                </div>
                <div className="profile-avatar-overlay">
                  <span className="profile-avatar-overlay-icon">📷</span>
                  <div className="profile-avatar-overlay-label">Change Photo</div>
                </div>
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
                  <div className="user-settings-field-input">
                    <input
                      type="text"
                      placeholder=" "
                      value={profileForm.firstName}
                      onChange={e => dispatch({ type: 'update-profile-form', value: { firstName: e.target.value } })}
                      id="edit-firstname"
                    />
                    <label htmlFor="edit-firstname">First Name</label>
                  </div>
                  <div className="user-settings-field-input">
                    <input
                      type="text"
                      placeholder=" "
                      value={profileForm.lastName}
                      onChange={e => dispatch({ type: 'update-profile-form', value: { lastName: e.target.value } })}
                      id="edit-lastname"
                    />
                    <label htmlFor="edit-lastname">Last Name</label>
                  </div>
                </div>
                <div className="button-group">
                  <button onClick={handleProfileSave}>Save Changes</button>
                  <button onClick={() => {
                    setStateField('editingProfile', false);
                    dispatch({ type: 'reset-from-user', user });
                  }} className="secondary">Cancel</button>
                </div>
              </div>
            ) : (
              <button onClick={() => setStateField('editingProfile', true)} className="secondary">
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
                  onChange={e => setStateField('emailCode', e.target.value)}
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
                  onChange={e => setStateField('twoFACode', e.target.value)}
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
              <button onClick={() => setStateField('showPasswordReset', !showPasswordReset)} className="secondary">
                {showPasswordReset ? 'Cancel Password Change' : 'Change Password'}
              </button>
              
              {showPasswordReset && (
                <div className="user-settings-input-group">
                  <div className="user-settings-field-input">
                    <input
                      type="password"
                      placeholder=" "
                      value={passwordForm.currentPassword}
                      onChange={e => dispatch({ type: 'update-password-form', value: { currentPassword: e.target.value } })}
                      id="current-password"
                    />
                    <label htmlFor="current-password">Current Password</label>
                  </div>
                  <div className="user-settings-field-input">
                    <input
                      type="password"
                      placeholder=" "
                      value={passwordForm.newPassword}
                      onChange={e => dispatch({ type: 'update-password-form', value: { newPassword: e.target.value } })}
                      id="new-password"
                    />
                    <label htmlFor="new-password">New Password</label>
                  </div>
                  <div className="user-settings-field-input">
                    <input
                      type="password"
                      placeholder=" "
                      value={passwordForm.confirmPassword}
                      onChange={e => dispatch({ type: 'update-password-form', value: { confirmPassword: e.target.value } })}
                      id="confirm-password"
                    />
                    <label htmlFor="confirm-password">Confirm New Password</label>
                  </div>
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
      
      {/* Profile Picture Modal */}
      {showProfilePictureModal && (
        <div className="profile-picture-modal">
          <div className="profile-picture-modal-content">
              <h3>Change Profile Picture</h3>
              
              <div className="profile-picture-options">
                <div className="profile-picture-option">
                  <button onClick={handleGenerateAvatar} className="secondary">
                    🎨 Generate Pixel Avatar
                  </button>
                  <p>Create a unique pixel art avatar</p>
                </div>
                
                <div className="profile-picture-divider">or</div>
                
                <div className="profile-picture-option">
                  <input
                    type="file"
                    id="profile-picture-upload"
                    accept="image/*"
                    onChange={handleFileSelect}
                    style={{ display: 'none' }}
                  />
                  <label htmlFor="profile-picture-upload" className="upload-label">
                    📁 Choose Image
                  </label>
                  {profilePictureFile && (
                    <div className="file-selected">
                      <span>Selected: {profilePictureFile.name}</span>
                      <button onClick={handleFileUpload}>Upload</button>
                    </div>
                  )}
                  <p>Upload your own image (max 5MB)</p>
                </div>
              </div>
              
              <button 
                onClick={() => {
                  setStateField('showProfilePictureModal', false);
                  setStateField('profilePictureFile', null);
                  setStateField('status', '');
                }} 
                className="secondary"
              >
                Cancel
              </button>
          </div>
        </div>
      )}
    </BaseModal>
  );
}
