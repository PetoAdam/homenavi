import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faArrowsRotate, faPuzzlePiece, faStore, faPowerOff, faPlug, faMusic, faMagnifyingGlass, faCubes, faKey, faRoute } from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import PageHeader from '../common/PageHeader/PageHeader';
import GlassCard from '../common/GlassCard/GlassCard';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import Button from '../common/Button/Button';
import RoleSelect from '../common/RoleSelect/RoleSelect';
import Toolbar from '../common/Toolbar/Toolbar';
import { useAuth } from '../../context/AuthContext';
import '../common/Toolbar/Toolbar.css';
import {
  getIntegrationRegistry,
  getIntegrationMarketplace,
  getIntegrationInstallStatus,
  installIntegration,
  reloadIntegrations,
  restartAllIntegrations,
  restartIntegration,
  setIntegrationSecrets,
  uninstallIntegration,
} from '../../services/integrationService';
import { getModalRoot } from '../common/Modal/modalRoot';
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
  const [installing, setInstalling] = useState({});
  const [uninstalling, setUninstalling] = useState({});
  const [marketplaceQuery, setMarketplaceQuery] = useState('');
  const [marketplaceShowInstalled, setMarketplaceShowInstalled] = useState(false);
  const [installStatus, setInstallStatus] = useState({});

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
  const filteredMarketplace = useMemo(() => {
    const term = marketplaceQuery.trim().toLowerCase();
    return marketplaceIntegrations.filter((entry) => {
      const isInstalled = Boolean(entry.installed || installedIds.has(entry.id));
      if (!marketplaceShowInstalled && isInstalled) return false;
      if (!term) return true;
      const name = String(entry.display_name || entry.id || '').toLowerCase();
      const id = String(entry.id || '').toLowerCase();
      const publisher = String(entry.publisher || '').toLowerCase();
      return name.includes(term) || id.includes(term) || publisher.includes(term);
    });
  }, [installedIds, marketplaceIntegrations, marketplaceQuery, marketplaceShowInstalled]);

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

  const isBundledIntegrationIconPath = (iconName) => {
    const raw = (iconName || '').toString().trim();
    if (!raw) return false;
    return raw.startsWith('/integrations/');
  };

  const totalPages = Math.max(1, Number(registry?.total_pages || 1));
  const pagedIntegrations = integrations;

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
    if (!accessToken || !isAdmin || activeTab !== 'marketplace') {
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

  const handleInstall = async (id) => {
    setInstalling((prev) => ({ ...prev, [id]: true }));
    setInstallStatus((prev) => ({
      ...prev,
      [id]: { id, stage: 'queued', progress: 5, message: 'Queued' },
    }));
    const res = await installIntegration(id);
    if (!res.success) {
      const detail = res.data?.detail ? ` (${res.data.detail})` : '';
      setError(`${res.error || 'Failed to install integration'}${detail}`);
      setInstalling((prev) => ({ ...prev, [id]: false }));
      return;
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
    setSelectedIntegration(match);
  }, [pendingSecretsId, registry]);

  const closeModal = () => {
    if (selectedIntegration?.id) {
      setSecretValidation((prev) => ({ ...prev, [selectedIntegration.id]: null }));
      setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
    }
    setSelectedIntegration(null);
    setPendingSecretsId(null);
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
    <div className="auth-modal-backdrop open" onClick={closeModal}>
      <div
        className="auth-modal-glass open integrations-admin-modal"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <button className="auth-modal-close" onClick={closeModal} type="button">×</button>
        <div className="auth-modal-content">
          {(() => {
            const iconRaw = selectedIntegration.icon || '';
            const fa = resolveFaIcon(iconRaw) || resolveFaIcon(selectedIntegration.id) || faPlug;
            return (
              <div className="integrations-admin-modal-header">
                <div className="integrations-admin-modal-title">
                  <div className="integrations-admin-modal-icon" aria-hidden="true">
                    {isBundledIntegrationIconPath(iconRaw) ? (
                      <img
                        src={iconRaw}
                        alt=""
                        onError={(e) => { e.currentTarget.style.display = 'none'; }}
                      />
                    ) : (
                      <FontAwesomeIcon icon={fa} />
                    )}
                  </div>
                  <div>
                    <div className="integrations-admin-modal-eyebrow">Integration</div>
                    <h3>{selectedIntegration.display_name || selectedIntegration.id}</h3>
                    <div className="integrations-admin-modal-sub">/{selectedIntegration.id}</div>
                  </div>
                </div>
                <div className="integrations-admin-modal-meta">
                  <span className="integrations-admin-badge">{selectedIntegration.widgets?.length || 0} widgets</span>
                  <span className="integrations-admin-badge">{Array.isArray(selectedIntegration.secrets) ? selectedIntegration.secrets.length : 0} secrets</span>
                </div>
              </div>
            );
          })()}
          <div className="integrations-admin-modal-body">
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-modal-desc">
                {selectedIntegration.description || selectedIntegration.summary || 'No description provided yet.'}
              </div>
            </div>
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Manage</div>
              <div className="integrations-admin-item-actions integrations-admin-secret-actions">
                <Button
                  variant="secondary"
                  onClick={async () => {
                    setRestarting((prev) => ({ ...prev, [selectedIntegration.id]: true }));
                    const res = await restartIntegration(selectedIntegration.id);
                    if (!res.success) {
                      setError(res.error || 'Failed to restart integration');
                    } else {
                      setError('');
                    }
                    setRestarting((prev) => ({ ...prev, [selectedIntegration.id]: false }));
                  }}
                  disabled={restarting[selectedIntegration.id]}
                >
                  {restarting[selectedIntegration.id] ? 'Restarting…' : 'Restart integration'}
                </Button>
                <Button variant="secondary" disabled>Disable</Button>
                <Button
                  variant="secondary"
                  onClick={() => handleUninstall(selectedIntegration.id)}
                  disabled={uninstalling[selectedIntegration.id]}
                >
                  {uninstalling[selectedIntegration.id] ? 'Removing…' : 'Remove'}
                </Button>
              </div>
            </div>

            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Secrets</div>
              <div className="integrations-admin-card-subtitle">Write-only fields. Values are never read back.</div>
              {pendingSecretsId === selectedIntegration.id ? (
                <div className="integrations-admin-secret-callout">
                  This integration requires secrets. Please add them now to finish setup.
                </div>
              ) : null}
              {secretValidation[selectedIntegration.id]?.message ? (
                <div className="integrations-admin-secret-error">
                  {secretValidation[selectedIntegration.id].message}
                </div>
              ) : null}
              {secretActionStatus[selectedIntegration.id] ? (
                <div className="integrations-admin-install-status">
                  <div className="integrations-admin-install-meta">
                    <span>{secretActionStatus[selectedIntegration.id].message}</span>
                    <span>{secretActionStatus[selectedIntegration.id].progress}%</span>
                  </div>
                  <div className="integrations-admin-install-bar">
                    <span style={{ width: `${secretActionStatus[selectedIntegration.id].progress}%` }} />
                  </div>
                </div>
              ) : null}
              {normalizeSecrets(selectedIntegration.secrets).length ? (
                <div className="integrations-admin-secret-list">
                  {normalizeSecrets(selectedIntegration.secrets).map((spec) => {
                    const missing = secretValidation[selectedIntegration.id]?.missing?.includes(spec.key);
                    const errorNonce = secretValidation[selectedIntegration.id]?.nonce || 0;
                    const fieldKey = missing ? `${spec.key}-${errorNonce}` : spec.key;
                    return (
                    <label className={`integrations-admin-secret-field${missing ? ' integrations-admin-secret-field--error' : ''}`} key={fieldKey}>
                      <span>{spec.key}</span>
                      {spec.description ? (
                        <span className="integrations-admin-secret-desc">{spec.description}</span>
                      ) : null}
                      <input
                        type="password"
                        className={`integrations-admin-input${missing ? ' integrations-admin-input--error' : ''}`}
                        value={(secretValues[selectedIntegration.id] || {})[spec.key] || ''}
                        placeholder="Enter secret"
                        onChange={(e) => {
                          const nextValue = e.target.value;
                          setSecretValidation((prev) => {
                            const current = prev[selectedIntegration.id];
                            if (!current?.missing?.length) return prev;
                            const remaining = current.missing.filter((key) => key !== spec.key);
                            return {
                              ...prev,
                              [selectedIntegration.id]: remaining.length
                                ? { ...current, missing: remaining, message: 'Some required secrets are still missing.' }
                                : null,
                            };
                          });
                          setSecretValues((prev) => ({
                            ...prev,
                            [selectedIntegration.id]: {
                              ...(prev[selectedIntegration.id] || {}),
                              [spec.key]: nextValue,
                            },
                          }));
                        }}
                      />
                    </label>
                  );
                  })}
                </div>
              ) : (
                <div className="integrations-admin-empty">No secrets declared.</div>
              )}
              <div className="integrations-admin-item-actions">
                <Button
                  onClick={async () => {
                    const requiredSpecs = normalizeSecrets(selectedIntegration.secrets);
                    const valuesToSave = secretValues[selectedIntegration.id] || {};
                    const missing = requiredSpecs
                      .map((spec) => spec.key)
                      .filter((key) => String(valuesToSave[key] || '').trim() === '');
                    if (missing.length) {
                      setSecretValidation((prev) => ({
                        ...prev,
                        [selectedIntegration.id]: {
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
                      [selectedIntegration.id]: { message: 'Saving secrets', progress: 35 },
                    }));
                    setSaving((prev) => ({ ...prev, [selectedIntegration.id]: true }));
                    const result = await setIntegrationSecrets(selectedIntegration.id, filtered);
                    if (!result.success) {
                      setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
                      setError(result.error || 'Failed to save secrets');
                    } else {
                      setError('');
                      setSecretValues((prev) => ({ ...prev, [selectedIntegration.id]: {} }));
                      setSecretValidation((prev) => ({ ...prev, [selectedIntegration.id]: null }));
                      setPendingSecretsId((prev) => (prev === selectedIntegration.id ? null : prev));
                      setSecretActionStatus((prev) => ({
                        ...prev,
                        [selectedIntegration.id]: { message: 'Restarting integration', progress: 70 },
                      }));
                      setRestarting((prev) => ({ ...prev, [selectedIntegration.id]: true }));
                      const restartResult = await restartIntegration(selectedIntegration.id);
                      if (!restartResult.success) {
                        setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
                        setError(restartResult.error || 'Secrets saved but restart failed');
                      } else {
                        setError('');
                        setSecretActionStatus((prev) => ({
                          ...prev,
                          [selectedIntegration.id]: { message: 'Restarted', progress: 100 },
                        }));
                        const closeId = selectedIntegration.id;
                        setTimeout(() => {
                          setSecretActionStatus((prev) => ({ ...prev, [closeId]: null }));
                          setSecretValidation((prev) => ({ ...prev, [closeId]: null }));
                          setPendingSecretsId(null);
                          setSelectedIntegration(null);
                        }, 900);
                      }
                      setRestarting((prev) => ({ ...prev, [selectedIntegration.id]: false }));
                    }
                    setSaving((prev) => ({ ...prev, [selectedIntegration.id]: false }));
                  }}
                  disabled={saving[selectedIntegration.id]}
                >
                  {saving[selectedIntegration.id] ? 'Saving…' : 'Save & restart'}
                </Button>
                {normalizeSecrets(selectedIntegration.secrets).length ? (
                  <Button
                    variant="secondary"
                    onClick={() => {
                      setSecretActionStatus((prev) => ({ ...prev, [selectedIntegration.id]: null }));
                      setPendingSecretsId(null);
                      setSelectedIntegration(null);
                    }}
                  >
                    Set up later
                  </Button>
                ) : null}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  ) : null;

  const marketplaceModalElement = selectedMarketplaceIntegration ? (
    <div className="auth-modal-backdrop open" onClick={() => setSelectedMarketplaceIntegration(null)}>
      <div
        className="auth-modal-glass open integrations-admin-modal"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <button
          className="auth-modal-close"
          onClick={() => setSelectedMarketplaceIntegration(null)}
          type="button"
        >
          ×
        </button>
        <div className="auth-modal-content">
          {(() => {
            const iconRaw = selectedMarketplaceIntegration.icon || '';
            const fa = resolveFaIcon(iconRaw) || resolveFaIcon(selectedMarketplaceIntegration.id) || faPlug;
            return (
              <div className="integrations-admin-modal-header">
                <div className="integrations-admin-modal-title">
                  <div className="integrations-admin-modal-icon" aria-hidden="true">
                    {isBundledIntegrationIconPath(iconRaw) ? (
                      <img
                        src={iconRaw}
                        alt=""
                        onError={(e) => { e.currentTarget.style.display = 'none'; }}
                      />
                    ) : (
                      <FontAwesomeIcon icon={fa} />
                    )}
                  </div>
                  <div>
                    <div className="integrations-admin-modal-eyebrow">Marketplace</div>
                    <h3>{selectedMarketplaceIntegration.display_name || selectedMarketplaceIntegration.id}</h3>
                    <div className="integrations-admin-modal-sub">/{selectedMarketplaceIntegration.id}</div>
                  </div>
                </div>
                <div className="integrations-admin-modal-meta">
                  {selectedMarketplaceIntegration.version ? (
                    <span className="integrations-admin-badge">v{selectedMarketplaceIntegration.version}</span>
                  ) : null}
                  {selectedMarketplaceIntegration.publisher ? (
                    <span className="integrations-admin-badge">{selectedMarketplaceIntegration.publisher}</span>
                  ) : null}
                </div>
              </div>
            );
          })()}
          <div className="integrations-admin-modal-body">
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-modal-desc">
                {selectedMarketplaceIntegration.description || 'No description provided yet.'}
              </div>
            </div>
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Details</div>
              <div className="integrations-admin-marketplace-details">
                <div><strong>ID:</strong> {selectedMarketplaceIntegration.id}</div>
                {selectedMarketplaceIntegration.version ? (
                  <div><strong>Version:</strong> {selectedMarketplaceIntegration.version}</div>
                ) : null}
                {selectedMarketplaceIntegration.publisher ? (
                  <div><strong>Publisher:</strong> {selectedMarketplaceIntegration.publisher}</div>
                ) : null}
                {selectedMarketplaceIntegration.homepage ? (
                  <div><strong>Homepage:</strong> <a href={selectedMarketplaceIntegration.homepage} target="_blank" rel="noreferrer">{selectedMarketplaceIntegration.homepage}</a></div>
                ) : null}
              </div>
            </div>
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Install</div>
              <div className="integrations-admin-item-actions">
                <Button
                  variant={selectedMarketplaceIntegration.installed ? 'secondary' : 'primary'}
                  onClick={() => handleInstall(selectedMarketplaceIntegration.id)}
                  disabled={selectedMarketplaceIntegration.installed || installing[selectedMarketplaceIntegration.id]}
                >
                  {selectedMarketplaceIntegration.installed
                    ? 'Installed'
                    : (installing[selectedMarketplaceIntegration.id] ? 'Installing…' : 'Install')}
                </Button>
              </div>
              {installing[selectedMarketplaceIntegration.id] ? (
                <div className="integrations-admin-install-status">
                  <div className="integrations-admin-install-meta">
                    <span>{installStatus[selectedMarketplaceIntegration.id]?.message || 'Installing…'}</span>
                    <span>{installStatus[selectedMarketplaceIntegration.id]?.progress ?? 0}%</span>
                  </div>
                  <div className="integrations-admin-install-bar">
                    <span style={{ width: `${installStatus[selectedMarketplaceIntegration.id]?.progress ?? 10}%` }} />
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </div>
      </div>
    </div>
  ) : null;

  const modal = modalElement
    ? (typeof document !== 'undefined'
      ? createPortal(modalElement, getModalRoot())
      : modalElement)
    : null;
  const marketplaceModal = marketplaceModalElement
    ? (typeof document !== 'undefined'
      ? createPortal(marketplaceModalElement, getModalRoot())
      : marketplaceModalElement)
    : null;

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
        <>
          <GlassCard className="integrations-admin-card integrations-admin-card--stack" interactive={false}>
            <div className="integrations-admin-card-header">
              <div className="integrations-admin-section-heading">
                <div className="integrations-admin-section-title">
                  <FontAwesomeIcon icon={faPuzzlePiece} />
                  <span>Installed integrations</span>
                </div>
                <div className="integrations-admin-section-sub">Manage secrets, status, and lifecycle.</div>
              </div>
              <Toolbar
                className="integrations-admin-toolbar hn-toolbar--inline"
                right={(
                  <div className="integrations-admin-toolbar-actions">
                    <div className="hn-toolbar-group">
                      <Button variant="secondary" className="hn-toolbar-iconbtn" onClick={handleReload} disabled={reloading}>
                        <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
                        <span className="btn-label">{reloading ? 'Refreshing…' : 'Refresh'}</span>
                      </Button>
                    </div>
                    <div className="hn-toolbar-group">
                      <Button variant="secondary" className="hn-toolbar-iconbtn" onClick={handleRestartAll} disabled={restartingAll}>
                        <span className="btn-icon"><FontAwesomeIcon icon={faPowerOff} /></span>
                        <span className="btn-label">{restartingAll ? 'Restarting…' : 'Restart all'}</span>
                      </Button>
                    </div>
                  </div>
                )}
              />
            </div>
            <div className="integrations-admin-divider" />
          <form
            className="integrations-admin-toolbar-form"
            onSubmit={(e) => {
              e.preventDefault();
              setPage(1);
            }}
          >
            <input
              className="input integrations-admin-input"
              placeholder="Search integrations"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(1);
              }}
            />
            <RoleSelect
              value={`${pageSize}/page`}
              options={PageSizeOptions.map((n) => `${n}/page`)}
              onChange={(val) => {
                const size = parseInt(val.split('/')[0], 10);
                if (!Number.isNaN(size) && size !== pageSize) {
                  setPage(1);
                  setPageSize(size);
                }
              }}
            />
            <Button type="submit">
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlass} /></span>
              <span className="btn-label">Search</span>
            </Button>
          </form>
          <div className="integrations-admin-table-wrapper table-wrapper">
            <table className="table integrations-admin-table">
              <thead>
                <tr>
                  <th><FontAwesomeIcon icon={faPuzzlePiece} /> Name</th>
                  <th><FontAwesomeIcon icon={faPlug} /> ID</th>
                  <th><FontAwesomeIcon icon={faCubes} /> Widgets</th>
                  <th><FontAwesomeIcon icon={faKey} /> Secrets</th>
                  <th><FontAwesomeIcon icon={faRoute} /> Route</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {pagedIntegrations.length ? pagedIntegrations.map((integration) => (
                  <tr
                    key={integration.id}
                    className="integrations-admin-row"
                    onClick={() => setSelectedIntegration(integration)}
                  >
                    <td>
                      <div className="integrations-admin-cell">
                        {(() => {
                          const iconRaw = integration.icon || '';
                          const fa = resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug;
                          if (isBundledIntegrationIconPath(iconRaw)) {
                            return (
                              <img
                                className="integrations-admin-icon"
                                src={iconRaw}
                                alt=""
                                aria-hidden="true"
                                onError={(e) => { e.currentTarget.style.display = 'none'; }}
                              />
                            );
                          }
                          return <FontAwesomeIcon icon={fa} className="integrations-admin-icon" />;
                        })()}
                        <div>
                          <div className="integrations-admin-name">{integration.display_name || integration.id}</div>
                          <div className="integrations-admin-sub">/{integration.id}</div>
                        </div>
                      </div>
                    </td>
                    <td className="integrations-admin-muted">/{integration.id}</td>
                    <td>{integration.widgets?.length || 0}</td>
                    <td>{Array.isArray(integration.secrets) ? integration.secrets.length : 0}</td>
                    <td className="integrations-admin-muted">{integration.route || `/apps/${integration.id}`}</td>
                    <td>
                      <div className="integrations-admin-inline-actions" onClick={(e) => e.stopPropagation()}>
                        <Button
                          variant="secondary"
                          className="hn-toolbar-iconbtn"
                          onClick={async () => {
                            setRestarting((prev) => ({ ...prev, [integration.id]: true }));
                            const res = await restartIntegration(integration.id);
                            if (!res.success) {
                              setError(res.error || 'Failed to restart integration');
                            } else {
                              setError('');
                            }
                            setRestarting((prev) => ({ ...prev, [integration.id]: false }));
                          }}
                          disabled={restarting[integration.id]}
                        >
                          <span className="btn-icon"><FontAwesomeIcon icon={faPowerOff} /></span>
                          <span className="btn-label">{restarting[integration.id] ? 'Restarting…' : 'Restart'}</span>
                        </Button>
                        <Button
                          variant="secondary"
                          onClick={() => handleUninstall(integration.id)}
                          disabled={uninstalling[integration.id]}
                        >
                          <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                          <span className="btn-label">{uninstalling[integration.id] ? 'Removing…' : 'Remove'}</span>
                        </Button>
                      </div>
                    </td>
                  </tr>
                )) : (
                  <tr>
                    <td colSpan={6} className="integrations-admin-empty">No integrations found.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <div className="integrations-admin-mobile-list">
            {pagedIntegrations.map((integration) => (
              <div
                key={`${integration.id}-mobile`}
                className="integrations-admin-mobile-card"
                onClick={() => setSelectedIntegration(integration)}
                role="button"
                tabIndex={0}
              >
                <div className="integrations-admin-mobile-header">
                  <div className="integrations-admin-cell">
                    {(() => {
                      const iconRaw = integration.icon || '';
                      const fa = resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug;
                      if (isBundledIntegrationIconPath(iconRaw)) {
                        return (
                          <img
                            className="integrations-admin-icon"
                            src={iconRaw}
                            alt=""
                            aria-hidden="true"
                            onError={(e) => { e.currentTarget.style.display = 'none'; }}
                          />
                        );
                      }
                      return <FontAwesomeIcon icon={fa} className="integrations-admin-icon" />;
                    })()}
                    <div>
                      <div className="integrations-admin-name">{integration.display_name || integration.id}</div>
                      <div className="integrations-admin-sub">/{integration.id}</div>
                    </div>
                  </div>
                  <div className="integrations-admin-mobile-counts">
                    <span>{integration.widgets?.length || 0} widgets</span>
                    <span>{Array.isArray(integration.secrets) ? integration.secrets.length : 0} secrets</span>
                  </div>
                </div>
                <div className="integrations-admin-mobile-route">{integration.route || `/apps/${integration.id}`}</div>
                <div className="integrations-admin-inline-actions" onClick={(e) => e.stopPropagation()}>
                  <Button
                    variant="secondary"
                    className="hn-toolbar-iconbtn"
                    onClick={async () => {
                      setRestarting((prev) => ({ ...prev, [integration.id]: true }));
                      const res = await restartIntegration(integration.id);
                      if (!res.success) {
                        setError(res.error || 'Failed to restart integration');
                      } else {
                        setError('');
                      }
                      setRestarting((prev) => ({ ...prev, [integration.id]: false }));
                    }}
                    disabled={restarting[integration.id]}
                  >
                    <span className="btn-icon"><FontAwesomeIcon icon={faPowerOff} /></span>
                    <span className="btn-label">{restarting[integration.id] ? 'Restarting…' : 'Restart'}</span>
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => handleUninstall(integration.id)}
                    disabled={uninstalling[integration.id]}
                  >
                    <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                    <span className="btn-label">{uninstalling[integration.id] ? 'Removing…' : 'Remove'}</span>
                  </Button>
                </div>
              </div>
            ))}
          </div>

          <div className="integrations-admin-pagination">
            <Button variant="secondary" disabled={page <= 1} onClick={() => setPage(page - 1)}>Prev</Button>
            <span className="integrations-admin-muted">Page {page} of {totalPages}</span>
            <Button variant="secondary" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Next</Button>
          </div>
        </GlassCard>
        </>
      ) : (
        <GlassCard className="integrations-admin-card" interactive={false}>
          <div className="integrations-admin-card-title">Marketplace</div>
          <div className="integrations-admin-card-subtitle">Install and configure integrations in one place.</div>
          {marketplaceError ? <div className="integrations-admin-error">{marketplaceError}</div> : null}
          <form
            className="integrations-admin-toolbar-form integrations-admin-marketplace-toolbar"
            onSubmit={(e) => e.preventDefault()}
          >
            <input
              className="input integrations-admin-input"
              placeholder="Search marketplace"
              value={marketplaceQuery}
              onChange={(e) => setMarketplaceQuery(e.target.value)}
            />
            <label className="integrations-admin-filter">
              <input
                type="checkbox"
                checked={marketplaceShowInstalled}
                onChange={(e) => setMarketplaceShowInstalled(e.target.checked)}
              />
              <span>Show installed</span>
            </label>
            <Button type="submit">
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlass} /></span>
              <span className="btn-label">Filter</span>
            </Button>
          </form>
          {marketplaceLoading ? (
            <div className="integrations-admin-empty">Loading marketplace…</div>
          ) : filteredMarketplace.length ? (
            <div className="integrations-admin-marketplace-list">
              {filteredMarketplace.map((entry) => {
                const iconRaw = entry.icon || '';
                const fa = resolveFaIcon(iconRaw) || resolveFaIcon(entry.id) || faPlug;
                const isInstalled = Boolean(entry.installed || installedIds.has(entry.id));
                const status = installStatus[entry.id];
                return (
                  <div
                    className="integrations-admin-marketplace-item"
                    key={entry.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => setSelectedMarketplaceIntegration(entry)}
                  >
                    <div className="integrations-admin-marketplace-info">
                      {isBundledIntegrationIconPath(iconRaw) ? (
                        <img
                          className="integrations-admin-marketplace-icon"
                          src={iconRaw}
                          alt=""
                          aria-hidden="true"
                          onError={(e) => { e.currentTarget.style.display = 'none'; }}
                        />
                      ) : (
                        <FontAwesomeIcon icon={fa} className="integrations-admin-marketplace-icon" />
                      )}
                      <div className="integrations-admin-marketplace-meta">
                        <div className="integrations-admin-marketplace-title">
                          {entry.display_name || entry.id}
                        </div>
                        <div className="integrations-admin-marketplace-text">
                          {entry.description || 'No description provided yet.'}
                        </div>
                        <div className="integrations-admin-marketplace-sub">
                          <span>/{entry.id}</span>
                          {entry.version ? <span>· v{entry.version}</span> : null}
                          {entry.publisher ? <span>· {entry.publisher}</span> : null}
                        </div>
                      </div>
                    </div>
                    <div className="integrations-admin-marketplace-actions" onClick={(e) => e.stopPropagation()}>
                      <Button
                        variant={isInstalled ? 'secondary' : 'primary'}
                        onClick={() => handleInstall(entry.id)}
                        disabled={isInstalled || installing[entry.id]}
                      >
                        {isInstalled ? 'Installed' : (installing[entry.id] ? 'Installing…' : 'Install')}
                      </Button>
                    </div>
                    {installing[entry.id] ? (
                      <div className="integrations-admin-install-status">
                        <div className="integrations-admin-install-meta">
                          <span>{status?.message || 'Installing…'}</span>
                          <span>{status?.progress ?? 0}%</span>
                        </div>
                        <div className="integrations-admin-install-bar">
                          <span style={{ width: `${status?.progress ?? 10}%` }} />
                        </div>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="integrations-admin-empty">No marketplace integrations available.</div>
          )}
        </GlassCard>
      )}

      {modal}
      {marketplaceModal}
    </div>
  );
}
