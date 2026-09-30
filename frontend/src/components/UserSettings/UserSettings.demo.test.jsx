// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import UserSettings from './UserSettings';

const authServiceMocks = vi.hoisted(() => ({
  requestEmailVerify: vi.fn(),
  confirmEmailVerify: vi.fn(),
  request2FAEmail: vi.fn(),
  verify2FAEmail: vi.fn(),
  changePassword: vi.fn(),
  patchUser: vi.fn(),
  generateAvatar: vi.fn(),
  uploadProfilePicture: vi.fn(),
}));

const authContextMocks = vi.hoisted(() => ({
  useAuth: vi.fn(),
}));

vi.mock('../../services/authService', () => authServiceMocks);
vi.mock('../../context/AuthContext', () => authContextMocks);

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

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

function clickButtonByText(text) {
  const button = Array.from(document.querySelectorAll('button')).find((entry) => entry.textContent?.includes(text));
  if (!button) throw new Error(`Button not found: ${text}`);
  act(() => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
}

describe('UserSettings public demo guardrails', () => {
  const originalDemoMode = import.meta.env.VITE_DEMO_MODE;

  beforeEach(() => {
    import.meta.env.VITE_DEMO_MODE = '1';
    authContextMocks.useAuth.mockReturnValue({
      user: {
        id: 'demo-user',
        first_name: 'Demo',
        last_name: 'Visitor',
        email: 'demo+abc123@example.com',
        user_name: 'demo_abc123',
        email_confirmed: false,
        two_factor_enabled: false,
      },
      accessToken: 'demo-token',
      handleLogout: vi.fn(),
      refreshUser: vi.fn(),
    });
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
    if (typeof originalDemoMode === 'undefined') {
      delete import.meta.env.VITE_DEMO_MODE;
    } else {
      import.meta.env.VITE_DEMO_MODE = originalDemoMode;
    }
  });

  it('shows snackbar feedback and does not submit 2FA or password mutations', () => {
    const view = renderIntoDom(<UserSettings onClose={vi.fn()} />);

    clickButtonByText('Enable 2FA');
    expect(document.body.textContent).toContain('This action is disabled in the public demo');
    expect(authServiceMocks.request2FAEmail).not.toHaveBeenCalled();

    clickButtonByText('Change Password');

    const inputs = Array.from(document.querySelectorAll('input'));
    const currentPassword = inputs.find((entry) => entry.id === 'current-password');
    const newPassword = inputs.find((entry) => entry.id === 'new-password');
    const confirmPassword = inputs.find((entry) => entry.id === 'confirm-password');
    act(() => {
      currentPassword.value = 'current-password';
      currentPassword.dispatchEvent(new Event('input', { bubbles: true }));
      newPassword.value = 'DemoValid9!';
      newPassword.dispatchEvent(new Event('input', { bubbles: true }));
      confirmPassword.value = 'DemoValid9!';
      confirmPassword.dispatchEvent(new Event('input', { bubbles: true }));
    });

    clickButtonByText('Update Password');
    expect(document.body.textContent).toContain('This action is disabled in the public demo');
    expect(authServiceMocks.changePassword).not.toHaveBeenCalled();

    view.unmount();
  });
});