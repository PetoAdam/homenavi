export const integrationsAdminUiInitialState = {
  activeTab: 'installed',
  selectedIntegration: null,
  selectedMarketplaceIntegration: null,
  installedModalTab: 'about',
  pendingSecretsId: null,
  pendingPostInstallId: null,
};

export function integrationsAdminUiReducer(state, action) {
  switch (action.type) {
    case 'set-active-tab':
      return {
        ...state,
        activeTab: action.tab,
      };
    case 'open-installed-modal':
      return {
        ...state,
        selectedIntegration: action.integration,
        installedModalTab: action.tab || 'about',
      };
    case 'close-installed-modal':
      return {
        ...state,
        selectedIntegration: null,
        pendingSecretsId: null,
        installedModalTab: 'about',
      };
    case 'set-installed-modal-tab':
      return {
        ...state,
        installedModalTab: action.tab,
      };
    case 'set-selected-marketplace-integration':
      return {
        ...state,
        selectedMarketplaceIntegration: action.integration || null,
      };
    case 'set-pending-secrets-id':
      return {
        ...state,
        pendingSecretsId: action.id || null,
      };
    case 'set-pending-post-install-id':
      return {
        ...state,
        pendingPostInstallId: action.id || null,
      };
    default:
      return state;
  }
}
