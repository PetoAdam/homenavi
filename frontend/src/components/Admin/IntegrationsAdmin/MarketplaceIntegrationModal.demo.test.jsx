// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MarketplaceIntegrationModal from './MarketplaceIntegrationModal';

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

describe('MarketplaceIntegrationModal public demo guardrails', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  it('blocks install clicks in demo mode', () => {
    const onInstallIntegration = vi.fn();
    const onDemoBlocked = vi.fn();
    const view = renderIntoDom(
      <MarketplaceIntegrationModal
        integration={{ id: 'spotify', name: 'Spotify', downloads: 42 }}
        onClose={vi.fn()}
        onInstallIntegration={onInstallIntegration}
        installing={{}}
        installStatus={{}}
        installedIds={new Set()}
        resolveFaIcon={vi.fn(() => null)}
        getMarketplaceName={(entry) => entry.name}
        getMarketplacePublisher={() => 'Community'}
        getMarketplaceVersion={() => '1.0.0'}
        formatDownloads={(value) => String(value)}
        demoModeEnabled
        onDemoBlocked={onDemoBlocked}
      />
    );

    const installButton = Array.from(document.querySelectorAll('button')).find((entry) => entry.textContent?.includes('Install'));
    act(() => {
      installButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(onDemoBlocked).toHaveBeenCalledTimes(1);
    expect(onInstallIntegration).not.toHaveBeenCalled();

    view.unmount();
  });
});