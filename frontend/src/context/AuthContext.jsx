import React, { createContext, useContext, useState, useEffect } from 'react';
import { login, finish2FA, signup, refreshToken, logout, getMe, request2FAEmail } from '../services/authService';

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
  useEffect(() => {
    if (!accessToken && refreshTokenValue) {
      (async () => {
        const res = await refreshToken(refreshTokenValue);
        if (res.success) {
          setAccessToken(res.accessToken);
          setRefreshTokenValue(res.refreshToken);
          localStorage.setItem('refreshToken', res.refreshToken);
        } else {
          setAccessToken(null);
          setRefreshTokenValue(null);
          localStorage.removeItem('refreshToken');
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
      }
    }, 13 * 60 * 1000); // every 13 min
    return () => clearInterval(interval);
  }, [refreshTokenValue]);

  const handleLogin = async (email, password) => {
    setLoading(true);
    const resp = await login(email, password);
    setLoading(false);
    if (resp.twoFA) {
      setPendingUserId(resp.userId); // Store userId for 2FA
      
      // Auto-request 2FA code if it's email-based
      if (resp.type === 'email') {
        try {
          await request2FAEmail(resp.userId, null); // No access token needed for this endpoint
        } catch (error) {
          console.warn('Failed to auto-request 2FA email:', error);
        }
      }
      
      return resp;
    }
    if (resp.success && resp.accessToken) {
      setAccessToken(resp.accessToken);
      setRefreshTokenValue(resp.refreshToken);
      localStorage.setItem('refreshToken', resp.refreshToken);
      setPendingUserId(null); // Clear pending userId
      // Fetch user profile after login
      const me = await getMe(resp.accessToken);
      if (me.success && me.user) {
        me.user.avatar = me.user.profile_picture_url || null;
        setUser(me.user);
        return { success: true };
      }
      return { success: false, error: me.error };
    }
    return { success: false, error: resp.error };
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
      setPendingUserId(null); // Clear pending userId
      // Fetch user profile after login
      const me = await getMe(resp.accessToken);
      if (me.success && me.user) {
        me.user.avatar = me.user.profile_picture_url || null;
        setUser(me.user);
        return { success: true };
      }
      return { success: false, error: me.error };
    }
    return { success: false, error: resp.error };
  };

  const handleSignup = async (firstName, lastName, userName, email, password) => {
    setLoading(true);
    const resp = await signup(firstName, lastName, userName, email, password);
    setLoading(false);
    return resp;
  };

  const requestNew2FACode = async () => {
    if (!pendingUserId) return { success: false, error: "No pending 2FA request" };
    
    setLoading(true);
    try {
      await request2FAEmail(pendingUserId, null);
      setLoading(false);
      return { success: true };
    } catch (error) {
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
    await logout(refreshTokenValue);
    setAccessToken(null);
    setRefreshTokenValue(null);
    localStorage.removeItem('refreshToken');
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
