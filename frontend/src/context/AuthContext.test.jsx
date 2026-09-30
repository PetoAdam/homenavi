// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const { authServiceMocks, setAuthCookie, clearAuthCookie, setHttpAccessToken } = vi.hoisted(() => ({
  authServiceMocks: {
    login: vi.fn(),
    finish2FA: vi.fn(),
    signup: vi.fn(),
    bootstrapDemoSession: vi.fn(),
    refreshToken: vi.fn(),
    logout: vi.fn(),
    getMe: vi.fn(),
    request2FAEmail: vi.fn(),
  },
  setAuthCookie: vi.fn(),
  clearAuthCookie: vi.fn(),
  setHttpAccessToken: vi.fn(),
}));

vi.mock('../services/authService', () => authServiceMocks);
vi.mock('../services/authCookie', () => ({ setAuthCookie, clearAuthCookie }));
vi.mock('../services/httpClient', () => ({ setAccessToken: setHttpAccessToken }));

import { AuthProvider, useAuth } from './AuthContext';

function AuthSnapshot() {
  const auth = useAuth();
  return (
    <div
      data-access-token={auth.accessToken || ''}
      data-bootstrapping={auth.bootstrapping ? 'true' : 'false'}
      data-user-role={auth.user?.role || ''}
    />
  );
}

function renderIntoDom(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    container,
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe('AuthProvider demo bootstrap', () => {
  const originalDemoMode = import.meta.env.VITE_DEMO_MODE;

  beforeEach(() => {
    Object.values(authServiceMocks).forEach((fn) => fn.mockReset());
    setAuthCookie.mockReset();
    clearAuthCookie.mockReset();
    setHttpAccessToken.mockReset();
    window.localStorage.clear();
    document.body.innerHTML = '';
    import.meta.env.VITE_DEMO_MODE = '1';

    authServiceMocks.bootstrapDemoSession.mockResolvedValue({
      success: true,
      accessToken: 'demo-access',
      refreshToken: 'demo-refresh',
    });
    authServiceMocks.getMe.mockResolvedValue({
      success: true,
      user: { id: 'demo-user', role: 'resident', profile_picture_url: '' },
    });
  });

  afterEach(() => {
    document.body.innerHTML = '';
    if (typeof originalDemoMode === 'undefined') {
      delete import.meta.env.VITE_DEMO_MODE;
    } else {
      import.meta.env.VITE_DEMO_MODE = originalDemoMode;
    }
  });

  it('bootstraps a demo session on mount only when demo mode is enabled and no session exists', async () => {
    const view = renderIntoDom(
      <AuthProvider>
        <AuthSnapshot />
      </AuthProvider>
    );

    await flushEffects();
    await flushEffects();

    const snapshot = view.container.firstElementChild;
    expect(authServiceMocks.bootstrapDemoSession).toHaveBeenCalledTimes(1);
    expect(authServiceMocks.refreshToken).not.toHaveBeenCalled();
    expect(window.localStorage.getItem('refreshToken')).toBe('demo-refresh');
    expect(setAuthCookie).toHaveBeenCalledWith('demo-access');
    expect(snapshot?.getAttribute('data-access-token')).toBe('demo-access');
    expect(snapshot?.getAttribute('data-user-role')).toBe('resident');

    view.unmount();
  });

  it('skips demo bootstrap when a refresh token already exists', async () => {
    window.localStorage.setItem('refreshToken', 'existing-refresh');
    authServiceMocks.refreshToken.mockResolvedValue({
      success: true,
      accessToken: 'existing-access',
      refreshToken: 'existing-refresh-2',
    });

    const view = renderIntoDom(
      <AuthProvider>
        <AuthSnapshot />
      </AuthProvider>
    );

    await flushEffects();
    await flushEffects();

    expect(authServiceMocks.bootstrapDemoSession).not.toHaveBeenCalled();
    expect(authServiceMocks.refreshToken).toHaveBeenCalledWith('existing-refresh');

    view.unmount();
  });
});