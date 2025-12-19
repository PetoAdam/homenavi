import React, { createContext, useContext, useState, useEffect } from 'react';
import { login, finish2FA, signup, refreshToken, logout, getMe, request2FAEmail } from '../services/authService';
import { setAuthCookie, clearAuthCookie } from '../services/authCookie';

const AuthContext = createContext();

export function AuthProvider({ children }) {
  const [accessToken, setAccessToken] = useState(null);
  const [refreshTokenValue, setRefreshTokenValue] = useState(() => localStorage.getItem('refreshToken') || null);
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(false);
  const [pendingUserId, setPendingUserId] = useState(null); // Store userId during 2FA flow

  useEffect(() => {
    if (accessToken) {
      getMe(accessToken).then(res => {
        if (res.success && res.user) {
          res.user.avatar = res.user.profile_picture_url || null;
          setUser(res.user);
        } else {
          setUser(null);
        }
      });
    } else {
      setUser(null);
    }
  }, [accessToken]);

  // On mount, try to refresh access token if refresh token exists in localStorage
  // Also check for Google OAuth callback
  useEffect(() => {
    // Check for Google OAuth callback with tokens in URL
    const urlParams = new URLSearchParams(window.location.search);
    const accessTokenFromUrl = urlParams.get('access_token');
    const refreshTokenFromUrl = urlParams.get('refresh_token');
    const error = urlParams.get('error');
    const reason = urlParams.get('reason');
    
    if (accessTokenFromUrl && refreshTokenFromUrl) {
      // Handle successful Google OAuth callback
      setAccessToken(accessTokenFromUrl);
      setRefreshTokenValue(refreshTokenFromUrl);
      localStorage.setItem('refreshToken', refreshTokenFromUrl);

      // Ensure cookie is set for api-gateway (and websocket) auth
      setAuthCookie(accessTokenFromUrl);
      
      // Clear URL parameters
      window.history.replaceState({}, document.title, window.location.pathname);
      return;
    }
    
    if (error) {
      // Handle OAuth error
      console.error('OAuth error:', error);
      let errorMessage = 'Authentication failed';
      switch (error) {
        case 'oauth_cancelled':
          errorMessage = 'Google login was cancelled';
          break;
        case 'oauth_failed':
          errorMessage = 'Google login failed';
          break;
        case 'oauth_exchange_failed':
          errorMessage = 'Failed to exchange OAuth code';
          break;
        case 'account_locked':
          errorMessage = reason === 'admin_lock' ? 'Account locked by administrator' : 'Account is locked';
          break;
        case 'email_conflict':
          errorMessage = 'Email already registered with different Google account';
          break;
        case 'token_failed':
          errorMessage = 'Failed to generate authentication tokens';
          break;
        case 'user_creation_failed':
          errorMessage = 'Failed to create user account';
          break;
        case 'link_failed':
          errorMessage = 'Failed to link Google account';
          break;
      }
      
      // Clear URL parameters
      window.history.replaceState({}, document.title, window.location.pathname);

      // Surface OAuth errors to the UI (ProfileButton toast).
      // Use sessionStorage so the message survives reload and doesn't depend on effect ordering.
      try {
        const payload = { source: 'oauth', error, reason, message: errorMessage };
        sessionStorage.setItem('homenavi:auth:oauth_error', JSON.stringify(payload));
        window.dispatchEvent(new CustomEvent('homenavi:toast', { detail: payload }));
      } catch {
        // ignore
      }

      console.error('OAuth Error:', errorMessage);
      return;
    }

    // Regular token refresh logic
    if (!accessToken && refreshTokenValue) {
      (async () => {
        const res = await refreshToken(refreshTokenValue);
        if (res.success) {
          setAccessToken(res.accessToken);
          setRefreshTokenValue(res.refreshToken);
          localStorage.setItem('refreshToken', res.refreshToken);

          setAuthCookie(res.accessToken);
        } else {
          setAccessToken(null);
          setRefreshTokenValue(null);
          localStorage.removeItem('refreshToken');

          clearAuthCookie();
        }
      })();
    }
  }, []);

  // Auto-refresh access token
  useEffect(() => {
    if (!refreshTokenValue) return;
    localStorage.setItem('refreshToken', refreshTokenValue);
    const interval = setInterval(async () => {
      const res = await refreshToken(refreshTokenValue);
      if (res.success) {
        setAccessToken(res.accessToken);
        setRefreshTokenValue(res.refreshToken);
        localStorage.setItem('refreshToken', res.refreshToken);

        setAuthCookie(res.accessToken);
      }
    }, 13 * 60 * 1000); // every 13 min
    return () => clearInterval(interval);
  }, [refreshTokenValue]);

  const handleLogin = async (email, password) => {
    setLoading(true);
    const resp = await login(email, password);
    setLoading(false);
    
    // Handle 2FA flow (return full resp so caller sees twoFA flag)
    if (resp.twoFA) {
      setPendingUserId(resp.userId); // Store userId for 2FA
      return resp; // includes twoFA, userId, type
    }
    
    // Handle successful login
    if (resp.success && resp.accessToken) {
      setAccessToken(resp.accessToken);
      setRefreshTokenValue(resp.refreshToken);
      localStorage.setItem('refreshToken', resp.refreshToken);

      setAuthCookie(resp.accessToken);
      
      setPendingUserId(null); // Clear pending userId
      // Fetch user profile after login
      const me = await getMe(resp.accessToken);
      if (me.success && me.user) {
        me.user.avatar = me.user.profile_picture_url || null;
        setUser(me.user);
        return { success: true };
      }
      return { success: false, error: me.error || "Failed to fetch user profile" };
    }
    
  // Failure: return full response (may include lockoutRemaining, reason, unlockAt)
  return { ...resp, success: false };
  };

  const handle2FA = async (code) => {
    setLoading(true);
    const userId = pendingUserId;
    if (!userId) {
      setLoading(false);
      return { success: false, error: "No pending 2FA request" };
    }
    const resp = await finish2FA(userId, code);
    setLoading(false);
    
    if (resp.success && resp.accessToken) {
      setAccessToken(resp.accessToken);
      setRefreshTokenValue(resp.refreshToken);
      localStorage.setItem('refreshToken', resp.refreshToken);

      setAuthCookie(resp.accessToken);
      
      setPendingUserId(null); // Clear pending userId
      // Fetch user profile after login
      const me = await getMe(resp.accessToken);
      if (me.success && me.user) {
        me.user.avatar = me.user.profile_picture_url || null;
        setUser(me.user);
        return { success: true };
      }
      return { success: false, error: me.error || "Failed to fetch user profile" };
    }
    
    // Make sure error is always a string
    const errorMessage = typeof resp.error === 'string' ? resp.error : 
                        (resp.error?.message || resp.error?.error || "2FA verification failed");
    console.error('2FA failed:', errorMessage);
    return { success: false, error: errorMessage };
  };

  const handleSignup = async (firstName, lastName, userName, email, password) => {
    setLoading(true);
    const resp = await signup(firstName, lastName, userName, email, password);
    setLoading(false);
    return resp;
  };

  const requestNew2FACode = async () => {
    if (!pendingUserId) return { success: false, error: "No pending 2FA request" };
    
    // Prevent multiple simultaneous requests
    if (loading) return { success: false, error: "Request already in progress" };
    
    setLoading(true);
    try {
      await request2FAEmail(pendingUserId, null);
      setLoading(false);
      return { success: true };
    } catch {
      setLoading(false);
      return { success: false, error: "Failed to request new code" };
    }
  };

  const cancelLogin = () => {
    setPendingUserId(null);
    setLoading(false);
  };

  const handleLogout = async () => {
    setLoading(true);
    await logout(refreshTokenValue, accessToken);
    setAccessToken(null);
    setRefreshTokenValue(null);
    localStorage.removeItem('refreshToken');

    clearAuthCookie();
    
    setUser(null);
    setPendingUserId(null); // Clear pending userId
    setLoading(false);
  };

  // Add a function to refresh user data in AuthContext
  const refreshUser = async () => {
    if (accessToken) {
      const res = await getMe(accessToken);
      if (res.success && res.user) {
        res.user.avatar = res.user.profile_picture_url || null; // Ensure avatar is set
        setUser(res.user);
        return res.user;
      }
    }
    return null;
  };

  return (
    <AuthContext.Provider value={{ 
      accessToken, 
      refreshToken: refreshTokenValue, 
      user, 
      loading, 
      handleLogin, 
      handle2FA, 
      handleSignup, 
      handleLogout, 
      cancelLogin, 
      requestNew2FACode,
      refreshUser 
    }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
