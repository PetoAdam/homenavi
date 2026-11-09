// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import OAuthAuthorize from './OAuthAuthorize';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('OAuth authorization permissions', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('submits only selected permissions and prevents an empty selection', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ authenticated: true, client_name: 'Example app', scopes: ['home.devices.read', 'home.devices.write'] }) })
      .mockResolvedValueOnce({ ok: false, json: async () => ({ error_description: 'Try again' }) });
    vi.stubGlobal('fetch', fetchMock);
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => root.render(<OAuthAuthorize />));
    expect(container.textContent).toContain('Connect Example app');
    const inputs = container.querySelectorAll('input[type="checkbox"]');
    const approve = Array.from(container.querySelectorAll('button')).find((button) => button.textContent === 'Authorize MCP access');
    await act(async () => inputs[1].click());
    await act(async () => approve.click());
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ scopes: ['home.devices.read'] });
    await act(async () => inputs[0].click());
    expect(approve.disabled).toBe(true);
    act(() => root.unmount());
  });
});