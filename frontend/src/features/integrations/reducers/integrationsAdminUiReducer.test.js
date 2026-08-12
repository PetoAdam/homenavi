import { describe, expect, it } from 'vitest';
import {
  integrationsAdminUiInitialState,
  integrationsAdminUiReducer,
} from './integrationsAdminUiReducer';

describe('integrationsAdminUiReducer', () => {
  it('opens installed modal with provided tab', () => {
    const next = integrationsAdminUiReducer(integrationsAdminUiInitialState, {
      type: 'open-installed-modal',
      integration: { id: 'spotify' },
      tab: 'manage',
    });

    expect(next.selectedIntegration).toEqual({ id: 'spotify' });
    expect(next.installedModalTab).toBe('manage');
  });

  it('closes installed modal and resets pending secrets and tab', () => {
    const state = {
      ...integrationsAdminUiInitialState,
      selectedIntegration: { id: 'spotify' },
      pendingSecretsId: 'spotify',
      installedModalTab: 'manage',
    };

    const next = integrationsAdminUiReducer(state, { type: 'close-installed-modal' });

    expect(next.selectedIntegration).toBeNull();
    expect(next.pendingSecretsId).toBeNull();
    expect(next.installedModalTab).toBe('about');
  });

  it('updates active tab and pending ids independently', () => {
    const withTab = integrationsAdminUiReducer(integrationsAdminUiInitialState, {
      type: 'set-active-tab',
      tab: 'marketplace',
    });
    const withPending = integrationsAdminUiReducer(withTab, {
      type: 'set-pending-post-install-id',
      id: 'spotify',
    });

    expect(withPending.activeTab).toBe('marketplace');
    expect(withPending.pendingPostInstallId).toBe('spotify');
  });

  it('sets and clears selected marketplace integration', () => {
    const selected = integrationsAdminUiReducer(integrationsAdminUiInitialState, {
      type: 'set-selected-marketplace-integration',
      integration: { id: 'matter' },
    });
    const cleared = integrationsAdminUiReducer(selected, {
      type: 'set-selected-marketplace-integration',
      integration: null,
    });

    expect(selected.selectedMarketplaceIntegration).toEqual({ id: 'matter' });
    expect(cleared.selectedMarketplaceIntegration).toBeNull();
  });
});
