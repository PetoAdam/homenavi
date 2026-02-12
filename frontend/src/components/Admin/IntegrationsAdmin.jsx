import React, { useCallback, useEffect, useMemo, useState } from 'react';
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
import { useAuth } from '../../context/AuthContext';
import {
  getIntegrationRegistry,
  getIntegrationMarketplace,
  getIntegrationInstallStatus,
  installIntegration,
  incrementMarketplaceDownloads,
  reloadIntegrations,
  restartAllIntegrations,
  restartIntegration,
  setIntegrationSecrets,
  uninstallIntegration,
} from '../../services/integrationService';
import '../Auth/AuthModal/AuthModal.css';
import './IntegrationsAdmin.css';

export default function IntegrationsAdmin() {
  const { user, accessToken } = useAuth();
  const [registry, setRegistry] = useState(null);
  const [error, setError] = useState('');
  const [reloading, setReloading] = useState(false);
  const [secretValues, setSecretValues] = useState({});
  const [saving, setSaving] = useState({});
  const [activeTab, setActiveTab] = useState('installed');
  const [selectedIntegration, setSelectedIntegration] = useState(null);
  const [restartingAll, setRestartingAll] = useState(false);
  const [restarting, setRestarting] = useState({});
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [marketplace, setMarketplace] = useState(null);
  const [marketplaceLoading, setMarketplaceLoading] = useState(false);
  const [marketplaceError, setMarketplaceError] = useState('');
  const [pendingSecretsId, setPendingSecretsId] = useState(null);
  const [secretValidation, setSecretValidation] = useState({});
  const [secretActionStatus, setSecretActionStatus] = useState({});
  const [selectedMarketplaceIntegration, setSelectedMarketplaceIntegration] = useState(null);
  const [installedModalTab, setInstalledModalTab] = useState('about');
  const [installing, setInstalling] = useState({});
  const [uninstalling, setUninstalling] = useState({});
  const [marketplaceQuery, setMarketplaceQuery] = useState('');
  const [marketplaceShowInstalled, setMarketplaceShowInstalled] = useState(false);
  const [installStatus, setInstallStatus] = useState({});
  const [marketplaceMode, setMarketplaceMode] = useState('discover');
  const [marketplaceFilter, setMarketplaceFilter] = useState('all');
  const [marketplaceSort, setMarketplaceSort] = useState('trending');

  const isAdmin = user?.role === 'admin';

  useEffect(() => {
    let alive = true;
    if (!accessToken || !isAdmin) {
      setRegistry(null);
      return () => { alive = false; };
    }
    const handle = setTimeout(() => {
      (async () => {
        const res = await getIntegrationRegistry({ q: query, page, pageSize });
        if (!alive) return;
        if (!res.success) {
          setError(res.error || 'Failed to load integrations');
          setRegistry(null);
          return;
        }
        setError('');
        setRegistry(res.data);
      })();
    }, query ? 300 : 0);
    return () => {
      alive = false;
      clearTimeout(handle);
    };
  }, [accessToken, isAdmin, query, page, pageSize]);

  const integrations = registry?.integrations || [];
  const installedCount = registry?.total ?? integrations.length;
  const PageSizeOptions = [10, 20, 50, 100];
  const installedIds = useMemo(() => new Set(integrations.map((integration) => integration.id)), [integrations]);
  const marketplaceIntegrations = marketplace?.integrations || [];
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

  const resolveFaIcon = (iconName) => {
    const key = normalizeIconKey(iconName);
    if (!key) return null;
    const faKey = key.startsWith('fa:') ? key.slice('fa:'.length).trim() : key;
    return FA_ICON_MAP[faKey] || null;
  };

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
    setMarketplaceLoading(true);
    const res = await getIntegrationMarketplace();
    if (!res.success) {
      setMarketplaceError(res.error || 'Failed to load marketplace');
      setMarketplace(null);
      setMarketplaceLoading(false);
      return false;
    }
    setMarketplaceError('');
    setMarketplace(res.data);
    setMarketplaceLoading(false);
    return true;
  }, []);

  const notifyIntegrationsUpdated = useCallback(() => {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new Event('homenavi:integrations-updated'));
    }
  }, []);

  useEffect(() => {
    let alive = true;
    if (!accessToken || !isAdmin) {
      return () => { alive = false; };
    }
    const shouldFetch = activeTab === 'marketplace' || (!marketplace && activeTab === 'installed');
    if (!shouldFetch) {
      return () => { alive = false; };
    }
    setMarketplaceLoading(true);
    (async () => {
      const res = await getIntegrationMarketplace();
      if (!alive) return;
      if (!res.success) {
        setMarketplaceError(res.error || 'Failed to load marketplace');
        setMarketplace(null);
      } else {
        setMarketplaceError('');
        setMarketplace(res.data);
      }
      setMarketplaceLoading(false);
    })();
    return () => { alive = false; };
  }, [accessToken, activeTab, isAdmin]);

  const refreshRegistryWithRetry = async (maxAttempts = 6, delayMs = 700, allowEmpty = false) => {
    const previousCount = registry?.integrations?.length || 0;
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      const refreshed = await getIntegrationRegistry({ q: query, page, pageSize });
      if (refreshed.success) {
        const nextList = Array.isArray(refreshed.data?.integrations) ? refreshed.data.integrations : [];
        if (nextList.length === 0 && previousCount > 0 && !allowEmpty) {
          setError('Integrations registry returned empty. Check installed.yaml and integration-proxy mounts.');
          return false;
        }
        setRegistry(refreshed.data);
        return true;
      }
      // eslint-disable-next-line no-await-in-loop
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
    return false;
  };

  const handleReload = async () => {
    setReloading(true);
    const res = await reloadIntegrations();
    if (!res.success) {
      setError(res.error || 'Failed to refresh integrations');
      setReloading(false);
      return;
    }
    setError('');
    await new Promise((resolve) => setTimeout(resolve, 600));
    await refreshRegistryWithRetry();
    notifyIntegrationsUpdated();
    setReloading(false);
  };

  const handleRestartAll = async () => {
    setRestartingAll(true);
    const res = await restartAllIntegrations();
    if (!res.success) {
      setError(res.error || 'Failed to restart integrations');
    } else {
      setError('');
    }
    setRestartingAll(false);
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
      };
    setInstalling((prev) => ({ ...prev, [id]: true }));
    setInstallStatus((prev) => ({
      ...prev,
      [id]: { id, stage: 'queued', progress: 5, message: 'Queued' },
    }));
    const res = await installIntegration(id, upstream, composePayload);
    if (!res.success) {
      const detail = res.data?.detail ? ` (${res.data.detail})` : '';
      setError(`${res.error || 'Failed to install integration'}${detail}`);
      setInstalling((prev) => ({ ...prev, [id]: false }));
      return;
    }
    if (typeof entryOrId !== 'string') {
      incrementMarketplaceDownloads(id).catch(() => {});
      setMarketplace((prev) => {
        if (!prev || !Array.isArray(prev.integrations)) return prev;
        const nextList = prev.integrations.map((entry) => {
          if (entry?.id !== id) return entry;
          const nextDownloads = Number(entry.downloads || 0) + 1;
          return { ...entry, downloads: nextDownloads };
        });
        return { ...prev, integrations: nextList };
      });
      setSelectedMarketplaceIntegration((prev) => {
        if (!prev || prev.id !== id) return prev;
        return { ...prev, downloads: Number(prev.downloads || 0) + 1 };
      });
    }
    setError('');
    await refreshRegistryWithRetry();
    await refreshMarketplace();
    setInstalling((prev) => ({ ...prev, [id]: false }));
    setPendingSecretsId(id);
    notifyIntegrationsUpdated();
  };

  const handleUninstall = async (id) => {
    setUninstalling((prev) => ({ ...prev, [id]: true }));
    const res = await uninstallIntegration(id);
    if (!res.success) {
      const detail = res.data?.detail ? ` (${res.data.detail})` : '';
      setError(`${res.error || 'Failed to uninstall integration'}${detail}`);
      setUninstalling((prev) => ({ ...prev, [id]: false }));
      return;
    }
    setError('');
    setRegistry((prev) => {
      if (!prev) return prev;
      const nextList = Array.isArray(prev.integrations) ? prev.integrations.filter((integration) => integration.id !== id) : [];
      const nextTotal = typeof prev.total === 'number' ? Math.max(0, prev.total - 1) : undefined;
      return { ...prev, integrations: nextList, ...(nextTotal !== undefined ? { total: nextTotal } : {}) };
    });
    if (selectedIntegration?.id === id) {
      setSelectedIntegration(null);
    }
    await refreshRegistryWithRetry(6, 700, true);
    await refreshMarketplace();
    setUninstalling((prev) => ({ ...prev, [id]: false }));
    notifyIntegrationsUpdated();
  };

  useEffect(() => {
    const activeIds = Object.entries(installing)
      .filter(([, active]) => active)
      .map(([id]) => id);
    if (!activeIds.length) return undefined;
    let cancelled = false;
    const poll = async () => {
      await Promise.all(activeIds.map(async (id) => {
        const res = await getIntegrationInstallStatus(id);
        if (!res.success || cancelled) return;
        setInstallStatus((prev) => ({ ...prev, [id]: res.data }));
      }));
    };
    poll();
    const handle = setInterval(poll, 1500);
    return () => {
      cancelled = true;
      clearInterval(handle);
    };
  }, [installing]);

  const openIntegrationModal = useCallback((integration, tab = 'about') => {
    const market = marketplaceById.get(integration.id);
    const next = market
      ? { ...integration, description: market.description || integration.description, images: market.images || integration.images, marketplace: market }
      : integration;
    setSelectedIntegration(next);
    setInstalledModalTab(tab);
  }, [marketplaceById]);

  useEffect(() => {
    if (!pendingSecretsId || !registry) return;
    const match = (registry.integrations || []).find((integration) => integration.id === pendingSecretsId);
    if (!match) return;
    const secretsRequired = Array.isArray(match.secrets) && match.secrets.length > 0;
    if (!secretsRequired) {
      setPendingSecretsId(null);
      return;
    }
    setActiveTab('installed');
    openIntegrationModal(match, 'manage');
  }, [pendingSecretsId, registry, openIntegrationModal]);

  const closeModal = () => {
    if (selectedIntegration?.id) {
      setSecretValidation((prev) => ({ ...prev, [selectedIntegration.id]: null }));
      setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
    }
    setSelectedIntegration(null);
    setPendingSecretsId(null);
    setInstalledModalTab('about');
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
    setRestarting((prev) => ({ ...prev, [id]: true }));
    const res = await restartIntegration(id);
    if (!res.success) {
      setError(res.error || 'Failed to restart integration');
    } else {
      setError('');
    }
    setRestarting((prev) => ({ ...prev, [id]: false }));
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
      setPendingSecretsId((prev) => (prev === id ? null : prev));
      setSecretActionStatus((prev) => ({
        ...prev,
        [id]: { message: 'Restarting integration', progress: 70 },
      }));
      setRestarting((prev) => ({ ...prev, [id]: true }));
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
          setPendingSecretsId(null);
          setSelectedIntegration(null);
        }, 900);
      }
      setRestarting((prev) => ({ ...prev, [id]: false }));
    }
    setSaving((prev) => ({ ...prev, [id]: false }));
  };

  const handleSetupLater = () => {
    if (!selectedIntegration?.id) return;
    setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
    setPendingSecretsId(null);
    setSelectedIntegration(null);
    setInstalledModalTab('about');
  };

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
      onTabChange={setInstalledModalTab}
      onClose={closeModal}
      onRestartIntegration={handleRestartIntegration}
      onUninstallIntegration={handleUninstall}
      restarting={restarting}
      uninstalling={uninstalling}
      normalizeSecrets={normalizeSecrets}
      pendingSecretsId={pendingSecretsId}
      secretValidation={secretValidation}
      secretActionStatus={secretActionStatus}
      secretValues={secretValues}
      onSecretChange={handleSecretChange}
      onSaveSecrets={handleSaveSecrets}
      saving={saving}
      onSetupLater={handleSetupLater}
      resolveFaIcon={resolveFaIcon}
    />
  ) : null;

  const marketplaceModalElement = selectedMarketplaceIntegration ? (
    <MarketplaceIntegrationModal
      integration={selectedMarketplaceIntegration}
      onClose={() => setSelectedMarketplaceIntegration(null)}
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
            onClick={() => setActiveTab(tab.id)}
            type="button"
          >
            <FontAwesomeIcon icon={tab.icon} />
            <span className="integrations-admin-nav-label">
              {tab.label}{tab.id === 'installed' ? ` (${installedCount})` : ''}
            </span>
          </button>
        ))}
      </div>

      {error ? <div className="integrations-admin-error">{error}</div> : null}
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
          restarting={restarting}
          uninstalling={uninstalling}
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
          onSelectIntegration={setSelectedMarketplaceIntegration}
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
    </div>
  );
}
