// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import IntegrationsAdminReadonly from './IntegrationsAdminReadonly';

vi.mock('../../context/AuthContext', () => ({
  useAuth: () => ({
    user: { role: 'admin' },
    accessToken: 'token',
  }),
}));

vi.mock('../../features/integrations/hooks/useIntegrationQueries', () => ({
  useIntegrationRegistryQuery: () => ({
    isLoading: false,
    error: null,
    data: {
      total: 1,
      integrations: [{
        id: 'spotify',
        display_name: 'Spotify',
        installed_version: '1.2.3',
        latest_version: '1.3.0',
        update_available: true,
        widgets: [{ id: 'player' }],
        secrets: [{ key: 'CLIENT_SECRET' }],
        route: '/apps/spotify',
        description: 'Music integration',
      }],
    },
  }),
  useIntegrationMarketplaceQuery: () => ({
    isLoading: false,
    error: null,
    data: {
      integrations: [{
        id: 'spotify',
        name: 'Spotify',
        version: '1.3.0',
        description: 'Marketplace description',
        publisher: 'Homenavi',
        downloads: 1200,
        featured: true,
        verified: true,
      }],
    },
  }),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderIntoDom(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('IntegrationsAdminReadonly', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  it('renders integration data without mutation actions', () => {
    const view = renderIntoDom(
      <MemoryRouter>
        <IntegrationsAdminReadonly />
      </MemoryRouter>
    );

    const buttonLabels = Array.from(document.querySelectorAll('button')).map((button) => button.textContent || '');

    expect(document.body.textContent).toContain('Integrations View');
    expect(document.body.textContent).toContain('Read-only mode');
    expect(document.body.textContent).toContain('Spotify');
    expect(document.body.textContent).toContain('/apps/spotify');
    expect(buttonLabels.some((label) => label.includes('Update'))).toBe(false);
    expect(buttonLabels.some((label) => label.includes('Remove'))).toBe(false);
    expect(buttonLabels.some((label) => label.includes('Restart'))).toBe(false);

    view.unmount();
  });
});