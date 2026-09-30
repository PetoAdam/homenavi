// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import InstalledIntegrationModal from './InstalledIntegrationModal';

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

function buildProps(overrides = {}) {
  return {
    integration: {
      id: 'spotify',
      display_name: 'Spotify',
      installed_version: '1.0.0',
      latest_version: '1.1.0',
      update_available: true,
      widgets: [],
      secrets: [{ key: 'CLIENT_SECRET', description: 'Secret' }],
    },
    activeTab: 'manage',
    onTabChange: vi.fn(),
    onClose: vi.fn(),
    onRestartIntegration: vi.fn(),
    onUninstallIntegration: vi.fn(),
    onUpdateIntegration: vi.fn(),
    onToggleAutoUpdate: vi.fn(),
    restarting: {},
    uninstalling: {},
    updating: {},
    installStatus: {},
    normalizeSecrets: (secrets) => secrets,
    pendingSecretsId: null,
    secretValidation: {},
    secretActionStatus: {},
    secretValues: { spotify: { CLIENT_SECRET: 'secret' } },
    onSecretChange: vi.fn(),
    onSaveSecrets: vi.fn(),
    saving: {},
    onSetupLater: vi.fn(),
    setupCapable: false,
    onOpenSetup: vi.fn(),
    resolveFaIcon: vi.fn(() => null),
    demoModeEnabled: true,
    onDemoBlocked: vi.fn(),
    ...overrides,
  };
}

describe('InstalledIntegrationModal public demo guardrails', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  it('blocks update, restart, remove, and save/restart clicks in demo mode', () => {
    const props = buildProps();
    const view = renderIntoDom(<InstalledIntegrationModal {...props} />);

    ['Update integration', 'Restart integration', 'Remove', 'Save & restart'].forEach((label) => {
      const button = Array.from(document.querySelectorAll('button')).find((entry) => entry.textContent?.includes(label));
      act(() => {
        button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      });
    });

    expect(props.onDemoBlocked).toHaveBeenCalledTimes(4);
    expect(props.onUpdateIntegration).not.toHaveBeenCalled();
    expect(props.onRestartIntegration).not.toHaveBeenCalled();
    expect(props.onUninstallIntegration).not.toHaveBeenCalled();
    expect(props.onSaveSecrets).not.toHaveBeenCalled();

    view.unmount();
  });
});