// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import UserSettings from './UserSettings';
import { listOAuthConsents, revokeOAuthConsent } from '../../services/authService';

const auth = vi.hoisted(() => ({
  user: { id: 'user-1', user_name: 'admin' },
  accessToken: 'test-token',
  handleLogout: vi.fn(),
  refreshUser: vi.fn(),
}));

vi.mock('../../context/AuthContext', () => ({ useAuth: () => auth }));
vi.mock('../../services/authService', async (importOriginal) => ({
  ...await importOriginal(),
  listOAuthConsents: vi.fn(),
  revokeOAuthConsent: vi.fn(),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('UserSettings connected applications', () => {
  let container;
  let root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    vi.resetAllMocks();
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  async function renderSettings() {
    await act(async () => root.render(<UserSettings onClose={vi.fn()} />));
    await act(async () => document.querySelector('.connected-apps-toggle').click());
  }

  it('shows the empty state when no applications are connected', async () => {
    listOAuthConsents.mockResolvedValue({ success: true, data: { consents: [] } });
    await renderSettings();

    expect(document.querySelector('.connected-apps-toggle').getAttribute('aria-expanded')).toBe('true');
    expect(document.querySelector('.connected-apps-card').textContent).toContain('No connected applications.');
  });

  it('shows permissions and allows retrying a failed disconnect', async () => {
    listOAuthConsents.mockResolvedValue({
      success: true,
      data: { consents: [{ client_id: 'client-1', client_name: 'Example app', resource: 'devices', scopes: ['devices:read'] }] },
    });
    revokeOAuthConsent.mockResolvedValueOnce({ success: false, error: 'Please retry.' })
      .mockResolvedValueOnce({ success: true });
    await renderSettings();

    expect(document.querySelector('.connected-app-row').textContent).toContain('Example app');
    expect(document.querySelector('.connected-app-scopes').textContent).toContain('devices:read');
    await act(async () => document.querySelector('.connected-app-row > button').click());
    expect(document.querySelector('.connected-app-action-error').textContent).toContain('Please retry.');
    await act(async () => document.querySelector('.connected-app-action-error button').click());
    expect(revokeOAuthConsent).toHaveBeenLastCalledWith('client-1', 'test-token');
    expect(document.querySelector('.connected-app-row')).toBeNull();
    expect(document.querySelector('.connected-apps-card').textContent).toContain('Connected application disconnected.');
  });
});