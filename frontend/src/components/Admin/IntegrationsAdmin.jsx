import React, { useCallback, useEffect, useMemo, useReducer, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faMusic,
  faPlug,
  faPuzzlePiece,
  faStore,
} from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import PageHeader from '../common/PageHeader/PageHeader';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import InstalledIntegrationsSection from './IntegrationsAdmin/InstalledIntegrationsSection';
import MarketplaceSection from './IntegrationsAdmin/MarketplaceSection';
import InstalledIntegrationModal from './IntegrationsAdmin/InstalledIntegrationModal';
import MarketplaceIntegrationModal from './IntegrationsAdmin/MarketplaceIntegrationModal';
import Snackbar from '../common/Snackbar/Snackbar';
import IntegrationIcon from '../common/IntegrationIcon/IntegrationIcon';
import {
  isSuccessfulOperationStatus,
  isTerminalOperationStatus,
} from './IntegrationsAdmin/integrationOperationStatus';
import { useAuth } from '../../context/AuthContext';
import {
  detectIntegrationSetupCapability,
  getIntegrationInstallStatus,
  getIntegrationUpdates,
  incrementMarketplaceDownloads,
  restartIntegration,
  setIntegrationSecrets,
} from '../../services/integrationService';
import {
  integrationMarketplaceQueryOptions,
  integrationRegistryQueryOptions,
  useIntegrationMarketplaceQuery,
  useIntegrationRegistryQuery,
} from '../../features/integrations/hooks/useIntegrationQueries';
import { useIntegrationMutations } from '../../features/integrations/hooks/useIntegrationMutations';
import { useIntegrationOperationPolling } from '../../features/integrations/hooks/useIntegrationOperationPolling';
import {
  integrationsAdminUiInitialState,
  integrationsAdminUiReducer,
} from '../../features/integrations/reducers/integrationsAdminUiReducer';
import {
  integrationsAdminOperationsInitialState,
  integrationsAdminOperationsReducer,
} from '../../features/integrations/reducers/integrationsAdminOperationsReducer';
import { queryKeys } from '../../state/queryKeys';
import { hasSetupUiPath, setupRouteForIntegration } from '../../utils/integrationSetup';
import '../Auth/AuthModal/AuthModal.css';
import './IntegrationsAdmin.css';

export default function IntegrationsAdmin() {
  const { user, accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [error, setError] = useState('');
  const [toast, setToast] = useState(null);
  const [secretValues, setSecretValues] = useState({});
  const [saving, setSaving] = useState({});
  const [uiState, dispatchUi] = useReducer(integrationsAdminUiReducer, integrationsAdminUiInitialState);
  const [operationsState, dispatchOperations] = useReducer(
    integrationsAdminOperationsReducer,
    integrationsAdminOperationsInitialState
  );
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [secretValidation, setSecretValidation] = useState({});
  const [secretActionStatus, setSecretActionStatus] = useState({});
  const [marketplaceQuery, setMarketplaceQuery] = useState('');
  const [marketplaceShowInstalled, setMarketplaceShowInstalled] = useState(false);
  const [marketplaceMode, setMarketplaceMode] = useState('discover');
  const [marketplaceFilter, setMarketplaceFilter] = useState('all');
  const [marketplaceSort, setMarketplaceSort] = useState('trending');
  const [setupCapabilities, setSetupCapabilities] = useState({});

  const {
    activeTab,
    selectedIntegration,
    selectedMarketplaceIntegration,
    installedModalTab,
    pendingSecretsId,
    pendingPostInstallId,
  } = uiState;

  const {
    reloading,
    restartingAll,
    restartAllTargets,
    installing,
    uninstalling,
    updating,
    restarting,
    installStatus,
  } = operationsState;

  const setActiveTabState = useCallback((tab) => {
    dispatchUi({ type: 'set-active-tab', tab });
  }, []);

  const setInstalledModalTabState = useCallback((tab) => {
    dispatchUi({ type: 'set-installed-modal-tab', tab });
  }, []);

  const setPendingSecretsIdState = useCallback((id) => {
    dispatchUi({ type: 'set-pending-secrets-id', id });
  }, []);

  const setPendingPostInstallIdState = useCallback((id) => {
    dispatchUi({ type: 'set-pending-post-install-id', id });
  }, []);

  const setSelectedMarketplaceIntegrationState = useCallback((integration) => {
    dispatchUi({ type: 'set-selected-marketplace-integration', integration });
  }, []);

  const isAdmin = user?.role === 'admin';

  useEffect(() => {
    const handle = setTimeout(() => {
      setDebouncedQuery(query);
    }, query ? 300 : 0);
    return () => {
      clearTimeout(handle);
    };
  }, [query]);

  const registryQuery = useIntegrationRegistryQuery(
    { q: debouncedQuery, page, pageSize },
    { enabled: Boolean(accessToken && isAdmin) }
  );

  const marketplaceDataQuery = useIntegrationMarketplaceQuery({
    enabled: Boolean(accessToken && isAdmin),
  });
  const {
    reloadMutation,
    restartAllMutation,
    installMutation,
    uninstallMutation,
    updateMutation,
    setAutoUpdateMutation,
  } = useIntegrationMutations({ q: debouncedQuery, page, pageSize });

  const registry = registryQuery.data || null;
  const marketplace = marketplaceDataQuery.data || null;
  const registryError = registryQuery.error?.message || '';
  const marketplaceError = marketplaceDataQuery.error?.message || '';
  const marketplaceLoading = marketplaceDataQuery.isLoading || marketplaceDataQuery.isFetching;

  const integrations = useMemo(() => registry?.integrations || [], [registry]);
  const installedCount = registry?.total ?? integrations.length;
  const PageSizeOptions = [10, 20, 50, 100];
  const installedIds = useMemo(() => new Set(integrations.map((integration) => integration.id)), [integrations]);
  const marketplaceIntegrations = useMemo(() => marketplace?.integrations || [], [marketplace]);
  const marketplaceById = useMemo(() => {
    const map = new Map();
    marketplaceIntegrations.forEach((entry) => {
      if (entry?.id) {
        map.set(entry.id, entry);
      }
    });
    return map;
  }, [marketplaceIntegrations]);
  const filteredMarketplace = useMemo(() => {
    const term = marketplaceQuery.trim().toLowerCase();
    let items = marketplaceIntegrations.filter((entry) => {
      const isInstalled = Boolean(entry.installed || installedIds.has(entry.id));
      if (!marketplaceShowInstalled && isInstalled) return false;
      if (!term) return true;
      const name = String(entry.name || entry.display_name || entry.id || '').toLowerCase();
      const id = String(entry.id || '').toLowerCase();
      const publisher = String(entry.publisher || '').toLowerCase();
      return name.includes(term) || id.includes(term) || publisher.includes(term);
    });
    if (marketplaceFilter === 'featured') {
      items = items.filter((entry) => entry.featured);
    } else if (marketplaceFilter === 'verified') {
      items = items.filter((entry) => entry.verified);
    } else if (marketplaceFilter === 'community') {
      items = items.filter((entry) => !entry.verified);
    }

    if (marketplaceSort === 'downloads') {
      items = [...items].sort((a, b) => (b.downloads || 0) - (a.downloads || 0));
    } else if (marketplaceSort === 'trending') {
      items = [...items].sort((a, b) => (b.trending_score || 0) - (a.trending_score || 0));
    } else if (marketplaceSort === 'version') {
      items = [...items].sort((a, b) => String(b.version || '').localeCompare(String(a.version || ''), undefined, { numeric: true }));
    } else {
      items = [...items].sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')));
    }

    return items;
  }, [installedIds, marketplaceIntegrations, marketplaceQuery, marketplaceShowInstalled, marketplaceFilter, marketplaceSort]);

  const featuredMarketplace = useMemo(
    () => marketplaceIntegrations.filter((entry) => entry.featured),
    [marketplaceIntegrations]
  );

  const mergedIntegrations = useMemo(() => integrations.map((integration) => {
    const market = marketplaceById.get(integration.id);
    if (!market) return integration;
    return {
      ...integration,
      description: market.description || integration.description,
      images: market.images || integration.images,
      marketplace: market,
    };
  }), [integrations, marketplaceById]);

  const integrationMetaById = useMemo(() => {
    const map = new Map();
    mergedIntegrations.forEach((entry) => {
      if (entry?.id) {
        map.set(entry.id, entry);
      }
    });
    marketplaceIntegrations.forEach((entry) => {
      if (!entry?.id || map.has(entry.id)) return;
      map.set(entry.id, entry);
    });
    return map;
  }, [mergedIntegrations, marketplaceIntegrations]);

  useEffect(() => {
    let cancelled = false;
    const ids = (integrations || []).map((integration) => integration.id).filter(Boolean);
    if (!ids.length) {
      setSetupCapabilities({});
      return () => {
        cancelled = true;
      };
    }
    (async () => {
      const results = await Promise.all(ids.map(async (id) => {
        const capability = await detectIntegrationSetupCapability(id);
        if (!capability?.success) return [id, undefined];
        return [id, Boolean(capability.capable)];
      }));
      if (cancelled) return;
      const next = {};
      results.forEach(([id, capable]) => {
        if (typeof capable === 'boolean') next[id] = capable;
      });
      setSetupCapabilities(next);
    })();
    return () => {
      cancelled = true;
    };
  }, [integrations]);

  const getMarketplaceName = (entry) => entry?.name || entry?.display_name || entry?.id || 'Integration';
  const getMarketplacePublisher = (entry) => entry?.publisher || 'Community';
  const getMarketplaceVersion = (entry) => {
    const raw = String(entry?.version || '').trim();
    if (!raw) return '1.0.0';
    return raw.startsWith('v') ? raw.slice(1) : raw;
  };

  const formatDownloads = (value) => {
    const count = Number(value || 0);
    if (!Number.isFinite(count)) return '0';
    if (count < 1000) return String(count);
    if (count < 1000000) return `${(count / 1000).toFixed(1).replace('.', ',')}k`;
    return `${(count / 1000000).toFixed(1).replace('.', ',')}M`;
  };

  const tabItems = [
    { id: 'installed', label: 'Installed', icon: faPuzzlePiece },
    { id: 'marketplace', label: 'Marketplace', icon: faStore },
  ];

  const FA_ICON_MAP = useMemo(() => ({
    spotify: faSpotify,
    music: faMusic,
    plug: faPlug,
  }), []);

  const normalizeIconKey = (iconName) => {
    const raw = (iconName || '').toString().trim();
    if (!raw) return '';
    return raw.toLowerCase();
  };

  const resolveFaIcon = useCallback((iconName) => {
    const key = normalizeIconKey(iconName);
    if (!key) return null;
    const faKey = key.startsWith('fa:') ? key.slice('fa:'.length).trim() : key;
    return FA_ICON_MAP[faKey] || null;
  }, [FA_ICON_MAP]);

  const buildOperationToast = useCallback((id, action) => {
    const meta = integrationMetaById.get(id) || {};
    const iconRaw = String(meta.icon || '').trim();
    const name = String(meta.display_name || meta.name || id || 'Integration').trim();
    let text = `${name} is now installed and ready to use.`;
    if (action === 'updated') {
      text = `${name} was updated successfully.`;
    } else if (action === 'restarted') {
      text = `${name} restarted successfully.`;
    }
    const fa = resolveFaIcon(iconRaw) || resolveFaIcon(id) || faPlug;
    return (
      <span className="integrations-admin-toast-content">
        <span className="integrations-admin-toast-icon" aria-hidden="true">
          <IntegrationIcon icon={iconRaw} faIcon={fa} fallbackIcon={faPlug} />
        </span>
        <span className="integrations-admin-toast-text">{text}</span>
      </span>
    );
  }, [integrationMetaById, resolveFaIcon]);

  const totalPages = Math.max(1, Number(registry?.total_pages || 1));
  const pagedIntegrations = mergedIntegrations;

  useEffect(() => {
    if (page > totalPages) {
      setPage(totalPages);
    }
  }, [page, totalPages]);

  const normalizeSecrets = (secrets) => {
    if (!Array.isArray(secrets)) return [];
    return secrets
      .map((entry) => {
        if (typeof entry === 'string') {
          return { key: entry, description: '' };
        }
        if (entry && typeof entry === 'object') {
          return {
            key: entry.key || entry.name || entry.id || '',
            description: entry.description || '',
          };
        }
        return null;
      })
      .filter((entry) => entry && entry.key);
  };

  const refreshMarketplace = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.integrations.marketplace() });
    const refreshed = await queryClient.fetchQuery(
      integrationMarketplaceQueryOptions({ enabled: true })
    );
    return Boolean(refreshed);
  }, [queryClient]);

  const notifyIntegrationsUpdated = useCallback(() => {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new Event('homenavi:integrations-updated'));
    }
  }, []);

  const refreshRegistryWithRetry = async (maxAttempts = 6, delayMs = 700, allowEmpty = false) => {
    const previousCount = registry?.integrations?.length || 0;
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      try {
        const refreshed = await queryClient.fetchQuery(
          integrationRegistryQueryOptions(
            { q: debouncedQuery, page, pageSize },
            { enabled: true }
          )
        );
        if (refreshed) {
          const nextList = Array.isArray(refreshed.integrations) ? refreshed.integrations : [];
          if (nextList.length === 0 && previousCount > 0 && !allowEmpty) {
            setError('Integrations registry returned empty. Check installed.yaml and integration-proxy mounts.');
            return false;
          }
          return true;
        }
      } catch {
        // Retry below.
      }
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
    return false;
  };

  const handleReload = async () => {
    dispatchOperations({ type: 'set-reloading', value: true });
    try {
      await reloadMutation.mutateAsync();
    } catch (err) {
      setError(err?.message || 'Failed to refresh integrations');
      dispatchOperations({ type: 'set-reloading', value: false });
      return;
    }
    setError('');
    await getIntegrationUpdates(true);
    await new Promise((resolve) => setTimeout(resolve, 600));
    await refreshRegistryWithRetry();
    notifyIntegrationsUpdated();
    dispatchOperations({ type: 'set-reloading', value: false });
  };

  const handleRestartAll = async () => {
    const ids = (registry?.integrations || []).map((integration) => integration.id).filter(Boolean);
    if (ids.length) {
      dispatchOperations({ type: 'queue-restart-all', ids });
    } else {
      dispatchOperations({ type: 'set-restarting-all', value: true });
    }
    try {
      await restartAllMutation.mutateAsync();
    } catch (err) {
      setError(err?.message || 'Failed to restart integrations');
      if (ids.length) {
        ids.forEach((id) => {
          dispatchOperations({ type: 'set-restarting', id, value: false });
        });
      }
      dispatchOperations({ type: 'set-restart-all-targets', ids: [] });
      dispatchOperations({ type: 'set-restarting-all', value: false });
      return;
    }
    setError('');
    if (!ids.length) {
      dispatchOperations({ type: 'set-restarting-all', value: false });
    }
  };

  const resolveInstallUpstream = (entry) => {
    if (!entry || !entry.id) return '';
    const explicit = String(entry.upstream || entry.marketplace?.upstream || '').trim();
    if (explicit) return explicit;
    const safeId = String(entry.id).trim();
    if (!/^[a-z0-9._-]+$/i.test(safeId)) return '';
    return `http://${safeId}:8099`;
  };

  const handleInstall = async (entryOrId) => {
    const id = typeof entryOrId === 'string' ? entryOrId : entryOrId?.id;
    if (!id) return;
    const upstream = typeof entryOrId === 'string' ? '' : resolveInstallUpstream(entryOrId);
    const composePayload = typeof entryOrId === 'string'
      ? null
      : {
        compose_file: entryOrId?.compose_file,
        version: entryOrId?.version,
      };
    dispatchOperations({ type: 'queue-install', id });
    try {
      await installMutation.mutateAsync({ id, upstream, composePayload });
    } catch (err) {
      setError(err?.message || 'Failed to install integration');
      dispatchOperations({ type: 'set-installing', id, value: false });
      return;
    }
    if (typeof entryOrId !== 'string') {
      incrementMarketplaceDownloads(id).catch(() => {});
      queryClient.setQueryData(queryKeys.integrations.marketplace(), (prev) => {
        if (!prev || !Array.isArray(prev.integrations)) return prev;
        const nextList = prev.integrations.map((entry) => {
          if (entry?.id !== id) return entry;
          const nextDownloads = Number(entry.downloads || 0) + 1;
          return { ...entry, downloads: nextDownloads };
        });
        return { ...prev, integrations: nextList };
      });
      if (selectedMarketplaceIntegration?.id === id) {
        setSelectedMarketplaceIntegrationState({
          ...selectedMarketplaceIntegration,
          downloads: Number(selectedMarketplaceIntegration.downloads || 0) + 1,
        });
      }
    }
    setError('');
    await refreshRegistryWithRetry();
    await refreshMarketplace();
    dispatchOperations({ type: 'set-installing', id, value: false });
    setPendingPostInstallIdState(id);
    notifyIntegrationsUpdated();
  };

  const handleUninstall = async (id) => {
    dispatchOperations({ type: 'set-uninstalling', id, value: true });
    try {
      await uninstallMutation.mutateAsync({ id });
    } catch (err) {
      setError(err?.message || 'Failed to uninstall integration');
      dispatchOperations({ type: 'set-uninstalling', id, value: false });
      return;
    }
    setError('');
    queryClient.setQueryData(queryKeys.integrations.registry({ q: debouncedQuery, page, pageSize }), (prev) => {
      if (!prev) return prev;
      const nextList = Array.isArray(prev.integrations) ? prev.integrations.filter((integration) => integration.id !== id) : [];
      const nextTotal = typeof prev.total === 'number' ? Math.max(0, prev.total - 1) : undefined;
      return { ...prev, integrations: nextList, ...(nextTotal !== undefined ? { total: nextTotal } : {}) };
    });
    if (selectedIntegration?.id === id) {
      dispatchUi({ type: 'close-installed-modal' });
    }
    await refreshRegistryWithRetry(6, 700, true);
    await refreshMarketplace();
    dispatchOperations({ type: 'set-uninstalling', id, value: false });
    notifyIntegrationsUpdated();
  };

  const handleInstallTerminal = useCallback((id, status) => {
    dispatchOperations({ type: 'set-installing', id, value: false });
    if (isSuccessfulOperationStatus(status)) {
      setToast(buildOperationToast(id, 'installed'));
    }
  }, [buildOperationToast]);

  const handleUpdateTerminal = useCallback((id, status) => {
    dispatchOperations({ type: 'set-updating', id, value: false });
    if (isSuccessfulOperationStatus(status)) {
      setToast(buildOperationToast(id, 'updated'));
    }
  }, [buildOperationToast]);

  const handleRestartTerminal = useCallback((id, status) => {
    dispatchOperations({ type: 'set-restarting', id, value: false });
    if (isSuccessfulOperationStatus(status) && !restartAllTargets.includes(id)) {
      setToast(buildOperationToast(id, 'restarted'));
    }
  }, [buildOperationToast, restartAllTargets]);

  useIntegrationOperationPolling({
    installing,
    updating,
    restarting,
    getStatus: getIntegrationInstallStatus,
    onStatus: (id, status) => {
      dispatchOperations({ type: 'set-install-status', id, status });
    },
    onInstallTerminal: handleInstallTerminal,
    onUpdateTerminal: handleUpdateTerminal,
    onRestartTerminal: handleRestartTerminal,
    isTerminalOperationStatus,
  });

  useEffect(() => {
    if (!restartingAll || !restartAllTargets.length) return;
    const hasActive = restartAllTargets.some((id) => Boolean(restarting[id]));
    if (hasActive) return;
    dispatchOperations({ type: 'set-restarting-all', value: false });
    dispatchOperations({ type: 'set-restart-all-targets', ids: [] });
    setToast('All integrations restarted successfully.');
  }, [restartingAll, restartAllTargets, restarting]);

  const openIntegrationModal = useCallback((integration, tab = 'about') => {
    const market = marketplaceById.get(integration.id);
    const next = market
      ? { ...integration, description: market.description || integration.description, images: market.images || integration.images, marketplace: market }
      : integration;
    dispatchUi({ type: 'open-installed-modal', integration: next, tab });
  }, [marketplaceById]);

  useEffect(() => {
    if (!pendingPostInstallId || !registry) return;
    const match = (registry.integrations || []).find((integration) => integration.id === pendingPostInstallId);
    if (!match) return;
    const secretsRequired = Array.isArray(match.secrets) && match.secrets.length > 0;
    if (secretsRequired) {
      setActiveTabState('installed');
      setPendingSecretsIdState(match.id);
      openIntegrationModal(match, 'manage');
      setPendingPostInstallIdState(null);
      return;
    }
    const setupCapable = hasSetupUiPath(match) || Boolean(setupCapabilities[match.id]);
    if (setupCapable) {
      const setupURL = setupRouteForIntegration(match);
      if (setupURL) {
        window.open(setupURL, '_blank', 'noopener,noreferrer');
      }
    }
    setPendingPostInstallIdState(null);
  }, [
    pendingPostInstallId,
    registry,
    openIntegrationModal,
    setupCapabilities,
    setActiveTabState,
    setPendingSecretsIdState,
    setPendingPostInstallIdState,
  ]);

  const closeModal = () => {
    if (selectedIntegration?.id) {
      setSecretValidation((prev) => ({ ...prev, [selectedIntegration.id]: null }));
      setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
    }
    dispatchUi({ type: 'close-installed-modal' });
  };

  const handleSearchSubmit = () => {
    setPage(1);
  };

  const handlePageSizeChange = (val) => {
    const size = parseInt(val.split('/')[0], 10);
    if (!Number.isNaN(size) && size !== pageSize) {
      setPage(1);
      setPageSize(size);
    }
  };

  const handleRestartIntegration = async (id) => {
    dispatchOperations({ type: 'queue-restart', id });
    const res = await restartIntegration(id);
    if (!res.success) {
      setError(res.error || 'Failed to restart integration');
      dispatchOperations({ type: 'set-restarting', id, value: false });
    } else {
      setError('');
    }
  };

  const handleUpdateIntegration = async (id) => {
    if (!id) return;
    dispatchOperations({
      type: 'queue-update',
      id,
      progress: installStatus[id]?.progress ?? 5,
      message: installStatus[id]?.message || 'Queued',
    });
    try {
      await updateMutation.mutateAsync({ id });
    } catch (err) {
      setError(err?.message || 'Failed to update integration');
      dispatchOperations({ type: 'set-updating', id, value: false });
      return;
    }
    setError('');
    await getIntegrationUpdates(true);
    await refreshRegistryWithRetry();
    dispatchOperations({ type: 'set-updating', id, value: false });
    notifyIntegrationsUpdated();
  };

  const handleToggleAutoUpdate = async (id, enabled) => {
    try {
      await setAutoUpdateMutation.mutateAsync({ id, enabled });
    } catch (err) {
      setError(err?.message || 'Failed to update auto-update policy');
      return;
    }
    setError('');
    queryClient.setQueryData(queryKeys.integrations.registry({ q: debouncedQuery, page, pageSize }), (prev) => {
      if (!prev || !Array.isArray(prev.integrations)) return prev;
      return {
        ...prev,
        integrations: prev.integrations.map((integration) => (
          integration.id === id
            ? { ...integration, auto_update: Boolean(enabled) }
            : integration
        )),
      };
    });
  };

  const handleSecretChange = (id, key, value) => {
    setSecretValidation((prev) => {
      const current = prev[id];
      if (!current?.missing?.length) return prev;
      const remaining = current.missing.filter((missingKey) => missingKey !== key);
      return {
        ...prev,
        [id]: remaining.length
          ? { ...current, missing: remaining, message: 'Some required secrets are still missing.' }
          : null,
      };
    });
    setSecretValues((prev) => ({
      ...prev,
      [id]: {
        ...(prev[id] || {}),
        [key]: value,
      },
    }));
  };

  const handleSaveSecrets = async (id) => {
    const integration = selectedIntegration && selectedIntegration.id === id
      ? selectedIntegration
      : (registry?.integrations || []).find((entry) => entry.id === id);
    if (!integration) return;

    const requiredSpecs = normalizeSecrets(integration.secrets);
    const valuesToSave = secretValues[id] || {};
    const missing = requiredSpecs
      .map((spec) => spec.key)
      .filter((key) => String(valuesToSave[key] || '').trim() === '');
    if (missing.length) {
      setSecretValidation((prev) => ({
        ...prev,
        [id]: {
          missing,
          message: `Please fill all required secrets: ${missing.join(', ')}`,
          nonce: Date.now(),
        },
      }));
      return;
    }

    const filtered = Object.fromEntries(
      Object.entries(valuesToSave).filter(([, v]) => String(v || '').trim() !== '')
    );
    if (!Object.keys(filtered).length) return;

    setSecretActionStatus((prev) => ({
      ...prev,
      [id]: { message: 'Saving secrets', progress: 35 },
    }));
    setSaving((prev) => ({ ...prev, [id]: true }));
    const result = await setIntegrationSecrets(id, filtered);
    if (!result.success) {
      setSecretActionStatus((prev) => ({ ...prev, [id]: null }));
      setError(result.error || 'Failed to save secrets');
    } else {
      setError('');
      setSecretValues((prev) => ({ ...prev, [id]: {} }));
      setSecretValidation((prev) => ({ ...prev, [id]: null }));
      if (pendingSecretsId === id) {
        setPendingSecretsIdState(null);
      }
      setSecretActionStatus((prev) => ({
        ...prev,
        [id]: { message: 'Restarting integration', progress: 70 },
      }));
      dispatchOperations({ type: 'set-restarting', id, value: true });
      const restartResult = await restartIntegration(id);
      if (!restartResult.success) {
        setSecretActionStatus((prev) => ({ ...prev, [id]: null }));
        setError(restartResult.error || 'Secrets saved but restart failed');
      } else {
        setError('');
        setSecretActionStatus((prev) => ({
          ...prev,
          [id]: { message: 'Restarted', progress: 100 },
        }));
        setTimeout(() => {
          setSecretActionStatus((prev) => ({ ...prev, [id]: null }));
          setSecretValidation((prev) => ({ ...prev, [id]: null }));
          dispatchUi({ type: 'close-installed-modal' });
        }, 900);
      }
      dispatchOperations({ type: 'set-restarting', id, value: false });
    }
    setSaving((prev) => ({ ...prev, [id]: false }));
  };

  const handleSetupLater = () => {
    if (!selectedIntegration?.id) return;
    setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
    dispatchUi({ type: 'close-installed-modal' });
  };

  const handleOpenSetup = useCallback((integration) => {
    const url = setupRouteForIntegration(integration);
    if (!url) return;
    window.open(url, '_blank', 'noopener,noreferrer');
  }, []);

  const handleMarketplaceModeChange = (item) => {
    setMarketplaceMode(item);
    if (item === 'downloads') {
      setMarketplaceSort('downloads');
    } else if (item === 'trending') {
      setMarketplaceSort('trending');
    } else {
      setMarketplaceSort('name');
    }
  };

  const displayError = error || registryError;

  if (!isAdmin) {
    return (
      <UnauthorizedView
        title="Admin"
        message="You need admin access to manage integrations."
        className="integrations-admin-page"
      />
    );
  }

  const modalElement = selectedIntegration ? (
    <InstalledIntegrationModal
      integration={selectedIntegration}
      activeTab={installedModalTab}
      onTabChange={setInstalledModalTabState}
      onClose={closeModal}
      onRestartIntegration={handleRestartIntegration}
      onUninstallIntegration={handleUninstall}
      onUpdateIntegration={handleUpdateIntegration}
      onToggleAutoUpdate={handleToggleAutoUpdate}
      restarting={restarting}
      uninstalling={uninstalling}
      updating={updating}
      installStatus={installStatus}
      normalizeSecrets={normalizeSecrets}
      pendingSecretsId={pendingSecretsId}
      secretValidation={secretValidation}
      secretActionStatus={secretActionStatus}
      secretValues={secretValues}
      onSecretChange={handleSecretChange}
      onSaveSecrets={handleSaveSecrets}
      saving={saving}
      onSetupLater={handleSetupLater}
      setupCapable={hasSetupUiPath(selectedIntegration)}
      onOpenSetup={() => handleOpenSetup(selectedIntegration)}
      resolveFaIcon={resolveFaIcon}
    />
  ) : null;

  const marketplaceModalElement = selectedMarketplaceIntegration ? (
    <MarketplaceIntegrationModal
      integration={selectedMarketplaceIntegration}
      onClose={() => setSelectedMarketplaceIntegrationState(null)}
      onInstallIntegration={handleInstall}
      installing={installing}
      installStatus={installStatus}
      installedIds={installedIds}
      resolveFaIcon={resolveFaIcon}
      getMarketplaceName={getMarketplaceName}
      getMarketplacePublisher={getMarketplacePublisher}
      getMarketplaceVersion={getMarketplaceVersion}
      formatDownloads={formatDownloads}
    />
  ) : null;

  const modal = modalElement;
  const marketplaceModal = marketplaceModalElement;

  return (
    <div className="integrations-admin-page">
      <PageHeader
        title="Integrations Admin"
        subtitle="Manage integrations and prepare for the marketplace."
      />

      <div className="integrations-admin-topnav">
        {tabItems.map((tab) => (
          <button
            key={tab.id}
            className={`integrations-admin-nav-btn${activeTab === tab.id ? ' active' : ''}`}
            onClick={() => setActiveTabState(tab.id)}
            type="button"
          >
            <FontAwesomeIcon icon={tab.icon} />
            <span className="integrations-admin-nav-label">
              {tab.label}{tab.id === 'installed' ? ` (${installedCount})` : ''}
            </span>
          </button>
        ))}
      </div>

      {displayError ? <div className="integrations-admin-error">{displayError}</div> : null}
      {activeTab === 'installed' ? (
        <InstalledIntegrationsSection
          integrations={pagedIntegrations}
          page={page}
          totalPages={totalPages}
          pageSize={pageSize}
          pageSizeOptions={PageSizeOptions}
          query={query}
          onQueryChange={(value) => {
            setQuery(value);
            setPage(1);
          }}
          onPageChange={setPage}
          onPageSizeChange={handlePageSizeChange}
          onSearchSubmit={handleSearchSubmit}
          onReload={handleReload}
          reloading={reloading}
          onRestartAll={handleRestartAll}
          restartingAll={restartingAll}
          onOpenIntegration={(integration) => openIntegrationModal(integration, 'about')}
          onOpenManage={(integration) => openIntegrationModal(integration, 'manage')}
          onRestartIntegration={handleRestartIntegration}
          onUninstallIntegration={handleUninstall}
          onUpdateIntegration={handleUpdateIntegration}
          onToggleAutoUpdate={handleToggleAutoUpdate}
          restarting={restarting}
          uninstalling={uninstalling}
          updating={updating}
          installStatus={installStatus}
          setupCapabilities={setupCapabilities}
          onOpenSetup={handleOpenSetup}
          resolveFaIcon={resolveFaIcon}
        />
      ) : (
        <MarketplaceSection
          marketplaceError={marketplaceError}
          marketplaceLoading={marketplaceLoading}
          featuredMarketplace={featuredMarketplace}
          filteredMarketplace={filteredMarketplace}
          marketplaceMode={marketplaceMode}
          marketplaceFilter={marketplaceFilter}
          marketplaceSort={marketplaceSort}
          marketplaceQuery={marketplaceQuery}
          marketplaceShowInstalled={marketplaceShowInstalled}
          onModeChange={handleMarketplaceModeChange}
          onFilterChange={setMarketplaceFilter}
          onSortChange={setMarketplaceSort}
          onQueryChange={setMarketplaceQuery}
          onShowInstalledChange={setMarketplaceShowInstalled}
          onSelectIntegration={setSelectedMarketplaceIntegrationState}
          onInstallIntegration={handleInstall}
          installing={installing}
          installStatus={installStatus}
          installedIds={installedIds}
          resolveFaIcon={resolveFaIcon}
          getMarketplaceName={getMarketplaceName}
          getMarketplacePublisher={getMarketplacePublisher}
          getMarketplaceVersion={getMarketplaceVersion}
          formatDownloads={formatDownloads}
        />
      )}

      {modal}
      {marketplaceModal}
      <Snackbar message={toast} onClose={() => setToast(null)} />
    </div>
  );
}
