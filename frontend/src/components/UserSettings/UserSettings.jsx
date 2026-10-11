import React, { useCallback, useEffect, useReducer } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCamera, faFolderOpen, faPlug, faRightFromBracket, faShieldHalved, faUser, faWandMagicSparkles } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../context/AuthContext';
import { 
  requestEmailVerify, 
  confirmEmailVerify,
  request2FAEmail,
  verify2FAEmail,
  setup2FATOTP,
  cancel2FASetup,
  verify2FATOTP,
  disable2FA,
  changePassword,
  listOAuthConsents,
  revokeOAuthConsent,
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
    twoFAMethod,
    totpQRCodeDataURL,
    recoveryCodes,
    showEmailCodeInput,
    show2FASetup,
    show2FACodeInput,
    show2FADisableForm,
    twoFADisableCode,
    editingProfile,
    profileForm,
    showPasswordReset,
    passwordForm,
    showProfilePictureModal,
    profilePictureFile,
    oauthConsents,
    oauthLoading,
    oauthError,
    disconnectClientID,
  } = state;

  const setStateField = (key, value) => dispatch({ type: 'set-field', key, value });

  // Sync local state with user data from AuthContext
  useEffect(() => {
    if (user) {
      dispatch({ type: 'reset-from-user', user });
    }
  }, [user]);

  const loadOAuthConsents = useCallback(async () => {
    if (!accessToken) {
      dispatch({ type: 'set-field', key: 'oauthConsents', value: [] });
      dispatch({ type: 'set-field', key: 'oauthError', value: 'Sign in again to view connected applications.' });
      return;
    }
    dispatch({ type: 'set-field', key: 'oauthLoading', value: true });
    dispatch({ type: 'set-field', key: 'oauthError', value: '' });
    try {
      const response = await listOAuthConsents(accessToken);
      if (response.success && Array.isArray(response.data?.consents)) {
        dispatch({ type: 'set-field', key: 'oauthConsents', value: response.data.consents });
      } else {
        dispatch({ type: 'set-field', key: 'oauthConsents', value: [] });
        dispatch({ type: 'set-field', key: 'oauthError', value: response.error || 'Could not load connected applications.' });
      }
    } catch {
      dispatch({ type: 'set-field', key: 'oauthConsents', value: [] });
      dispatch({ type: 'set-field', key: 'oauthError', value: 'Could not load connected applications.' });
    } finally {
      dispatch({ type: 'set-field', key: 'oauthLoading', value: false });
    }
  }, [accessToken]);

  useEffect(() => {
    void loadOAuthConsents();
  }, [accessToken, loadOAuthConsents]);

  const handleLogoutAndClose = async () => {
    await handleLogout();
    onClose();
  };

  const handle2FASetupCancel = async () => {
    setStateField('status', 'Cancelling 2FA setup...');
    const response = await cancel2FASetup(accessToken);
    if (!response.success) {
      setStateField('status', '❌ ' + (response.error || 'Failed to cancel 2FA setup'));
      return;
    }
    setStateField('twoFACode', '');
    setStateField('totpQRCodeDataURL', '');
    setStateField('show2FACodeInput', false);
    setStateField('show2FASetup', false);
    setStateField('status', '');
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
    setStateField('status', 'Setting up 2FA...');
    const resp = twoFAMethod === 'totp'
      ? await setup2FATOTP(accessToken)
      : await request2FAEmail(accessToken);
    if (resp.success) {
      if (twoFAMethod === 'totp') {
        const qrCodeDataURL = resp.data?.qr_code_data_url;
        if (!qrCodeDataURL) {
          setStateField('status', 'Failed to prepare authenticator setup.');
          return;
        }
        setStateField('totpQRCodeDataURL', qrCodeDataURL);
        setStateField('status', 'Scan the QR code with your authenticator app, then enter its code.');
      } else {
        setStateField('status', 'Check your email for the 2FA code.');
      }
      setStateField('show2FACodeInput', true);
    } else {
      setStateField('status', '❌ ' + (resp.error || 'Failed to setup 2FA'));
    }
  };

  const handle2FAVerify = async () => {
    if (!twoFACode.trim()) return;
    setStateField('status', 'Verifying 2FA...');
    const resp = twoFAMethod === 'totp'
      ? await verify2FATOTP(twoFACode.trim(), accessToken)
      : await verify2FAEmail(twoFACode.trim(), accessToken);
    if (resp.success) {
      setStateField('status', '✅ 2FA enabled successfully!');
      setStateField('recoveryCodes', Array.isArray(resp.data?.recovery_codes) ? resp.data.recovery_codes : []);
      setStateField('twoFACode', '');
      setStateField('show2FACodeInput', false);
      setStateField('totpQRCodeDataURL', '');
      // Refresh user data from backend to get updated two_factor_enabled status
      await refreshUser();
    } else {
      setStateField('status', '❌ ' + (resp.error || '2FA verification failed'));
    }
  };

  const handle2FADisable = async () => {
    if (!twoFADisableCode.trim()) return;
    setStateField('status', 'Disabling 2FA...');
    const response = await disable2FA(twoFADisableCode.trim(), accessToken);
    if (!response.success) {
      setStateField('status', '❌ ' + (response.error || 'Failed to disable 2FA'));
      return;
    }
    setStateField('status', '✅ 2FA disabled. You can now choose a new method.');
    setStateField('twoFADisableCode', '');
    setStateField('show2FADisableForm', false);
    setStateField('show2FASetup', false);
    await refreshUser();
  };

  const handleConsentRevoke = async (clientID) => {
    setStateField('disconnectClientID', clientID);
    const response = await revokeOAuthConsent(clientID, accessToken);
    if (response.success) {
      setStateField('oauthConsents', oauthConsents.filter((consent) => consent.client_id !== clientID));
      setStateField('status', '✅ Connected application removed.');
    } else {
      setStateField('status', '❌ ' + (response.error || 'Failed to remove connected application'));
    }
    setStateField('disconnectClientID', '');
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
    if (twoFAEnabled && !passwordForm.twoFactorCode.trim()) {
      setStateField('status', '❌ Enter your current 2FA code to update your password');
      return;
    }
    const resp = await changePassword(passwordForm.currentPassword, passwordForm.newPassword, passwordForm.twoFactorCode.trim(), accessToken);
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
              <span className="user-settings-card-icon"><FontAwesomeIcon icon={faUser} /></span>
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
                  <span className="profile-avatar-overlay-icon"><FontAwesomeIcon icon={faCamera} /></span>
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
              <span className="user-settings-card-icon"><FontAwesomeIcon icon={faShieldHalved} /></span>
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
            <section className="two-factor-section" aria-labelledby="two-factor-heading">
              <div className="two-factor-heading-row">
                <div>
                  <h4 id="two-factor-heading">Two-Factor Authentication</h4>
                  <p>{twoFAEnabled ? 'Your account requires an additional verification step.' : 'Protect sensitive account changes with a second factor.'}</p>
                </div>
                <span className={`status-indicator ${twoFAEnabled ? 'verified' : 'unverified'}`}>
                  {twoFAEnabled ? 'Enabled' : 'Disabled'}
                </span>
              </div>

              {!twoFAEnabled && !show2FACodeInput && !show2FASetup && (
                <button type="button" className="secondary" onClick={() => setStateField('show2FASetup', true)}>
                  Set up 2FA
                </button>
              )}

              {!twoFAEnabled && !show2FACodeInput && show2FASetup && (
                <div className="two-factor-setup-panel">
                  <span className="two-factor-label">Choose a verification method</span>
                  <div className="two-factor-methods" role="group" aria-label="Two-factor authentication method">
                    <button
                      type="button"
                      className={`two-factor-method ${twoFAMethod === 'totp' ? 'active' : ''}`}
                      onClick={() => setStateField('twoFAMethod', 'totp')}
                    >
                      <span>Authenticator app</span>
                      <small>Recommended</small>
                    </button>
                    <button
                      type="button"
                      className={`two-factor-method ${twoFAMethod === 'email' ? 'active' : ''}`}
                      onClick={() => setStateField('twoFAMethod', 'email')}
                    >
                      <span>Email code</span>
                      <small>Use your inbox</small>
                    </button>
                  </div>
                  <button onClick={handle2FASetup} className="two-factor-primary-action">
                    Continue with {twoFAMethod === 'totp' ? 'authenticator app' : 'email code'}
                  </button>
                  <button type="button" className="secondary two-factor-cancel-action" onClick={() => setStateField('show2FASetup', false)}>
                    Cancel
                  </button>
                </div>
              )}

            {twoFAEnabled && (
              <div className="security-action-row">
                <span>Your enrolled factor is required before this setting can change.</span>
                <button onClick={() => setStateField('show2FADisableForm', !show2FADisableForm)} className="secondary">
                  {show2FADisableForm ? 'Cancel' : 'Change or Disable 2FA'}
                </button>
              </div>
            )}

            {show2FADisableForm && (
              <div className="security-confirmation">
                <strong>Confirm with your current 2FA code</strong>
                <p>Disabling 2FA also removes its recovery codes. Enable a new method immediately if you are changing it.</p>
                <div className="user-settings-input-group">
                  <input
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    placeholder="Six-digit code"
                    value={twoFADisableCode}
                    onChange={e => setStateField('twoFADisableCode', e.target.value)}
                  />
                  <button onClick={handle2FADisable} className="danger" disabled={!twoFADisableCode.trim()}>Disable 2FA</button>
                </div>
              </div>
            )}

            {show2FACodeInput && !twoFAEnabled && (
              <div className="two-factor-setup-panel two-factor-verification-panel">
                <div className="two-factor-verification-copy">
                  <strong>{twoFAMethod === 'totp' ? 'Confirm your authenticator app' : 'Confirm your email code'}</strong>
                  <span>{twoFAMethod === 'totp' ? 'Scan the code, then enter the six-digit value from your app.' : 'Enter the six-digit code sent to your email address.'}</span>
                </div>
                {twoFAMethod === 'totp' && totpQRCodeDataURL && (
                  <img className="two-fa-qr-code" src={totpQRCodeDataURL} alt="Authenticator app setup QR code" />
                )}
                <div className="two-factor-code-row">
                  <input
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    placeholder="Six-digit code"
                    value={twoFACode}
                    onChange={e => setStateField('twoFACode', e.target.value)}
                  />
                  <button onClick={handle2FAVerify} disabled={!twoFACode.trim()}>
                    Confirm
                  </button>
                  <button type="button" className="secondary" onClick={handle2FASetupCancel}>
                    Cancel setup
                  </button>
                </div>
              </div>
            )}
            </section>
            
            {status && status.includes('2FA') && (
              <div className={`card-status ${
                status.includes('✅') ? 'success' : 
                status.includes('❌') ? 'error' : ''
              }`}>
                {status}
              </div>
            )}

            {recoveryCodes.length > 0 && (
              <div className="recovery-codes" role="status">
                <strong>Recovery codes</strong>
                <div className="recovery-code-list">
                  {recoveryCodes.map((code) => <code key={code}>{code}</code>)}
                </div>
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
                  {twoFAEnabled && (
                    <div className="user-settings-field-input">
                      <input
                        type="text"
                        inputMode="numeric"
                        autoComplete="one-time-code"
                        maxLength={6}
                        placeholder=" "
                        value={passwordForm.twoFactorCode}
                        onChange={e => dispatch({ type: 'update-password-form', value: { twoFactorCode: e.target.value } })}
                        id="password-two-factor-code"
                      />
                      <label htmlFor="password-two-factor-code">Current 2FA Code</label>
                    </div>
                  )}
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

          <div className="user-settings-card connected-apps-card">
            <h3><span className="user-settings-card-icon"><FontAwesomeIcon icon={faPlug} /></span>Connected Applications</h3>
            <p className="settings-supporting-text">Applications authorized to access Homenavi on your behalf.</p>
            {oauthLoading && <p className="settings-supporting-text">Loading connected applications...</p>}
            {!oauthLoading && oauthError && (
              <div className="card-status error" role="alert">
                {oauthError}
                <button type="button" className="secondary" onClick={() => void loadOAuthConsents()}>Try again</button>
              </div>
            )}
            {!oauthLoading && !oauthError && oauthConsents.length === 0 && <p className="settings-supporting-text">No connected applications. Authorize an MCP client to manage it here.</p>}
            {oauthConsents.map((consent) => (
              <div className="connected-app-row" key={`${consent.client_id}-${consent.resource}`}>
                <div>
                  <strong>{consent.client_id}</strong>

                  <span>{Array.isArray(consent.scopes) && consent.scopes.length > 0 ? consent.scopes.join(', ') : 'No scopes'}</span>
                </div>
                <button
                  className="danger"
                  onClick={() => handleConsentRevoke(consent.client_id)}
                  disabled={disconnectClientID === consent.client_id}
                >
                  {disconnectClientID === consent.client_id ? 'Disconnecting...' : 'Disconnect'}
                </button>
              </div>
            ))}
            {status && status.includes('Connected application') && <div className={`card-status ${status.includes('✅') ? 'success' : 'error'}`}>{status}</div>}
          </div>

          {/* Logout Card */}
          <div className="user-settings-card">
            <h3>
              <span className="user-settings-card-icon"><FontAwesomeIcon icon={faRightFromBracket} /></span>
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
                    <FontAwesomeIcon icon={faWandMagicSparkles} /> Generate Pixel Avatar
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
                    <FontAwesomeIcon icon={faFolderOpen} /> Choose Image
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
