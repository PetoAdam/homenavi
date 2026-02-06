import React, { useEffect, useMemo, useState } from 'react';
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
  reloadIntegrations,
  restartAllIntegrations,
  restartIntegration,
  setIntegrationSecrets,
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

  const refreshRegistryWithRetry = async (maxAttempts = 6, delayMs = 700) => {
    const previousCount = registry?.integrations?.length || 0;
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      const refreshed = await getIntegrationRegistry({ q: query, page, pageSize });
      if (refreshed.success) {
        const nextList = Array.isArray(refreshed.data?.integrations) ? refreshed.data.integrations : [];
        if (nextList.length === 0 && previousCount > 0) {
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

  const closeModal = () => setSelectedIntegration(null);

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
                <Button variant="secondary" disabled>Remove</Button>
              </div>
            </div>

            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Secrets</div>
              <div className="integrations-admin-card-subtitle">Write-only fields. Values are never read back.</div>
              {normalizeSecrets(selectedIntegration.secrets).length ? (
                <div className="integrations-admin-secret-list">
                  {normalizeSecrets(selectedIntegration.secrets).map((spec) => (
                    <label className="integrations-admin-secret-field" key={spec.key}>
                      <span>{spec.key}</span>
                      {spec.description ? (
                        <span className="integrations-admin-secret-desc">{spec.description}</span>
                      ) : null}
                      <input
                        type="password"
                        className="integrations-admin-input"
                        value={(secretValues[selectedIntegration.id] || {})[spec.key] || ''}
                        placeholder="Enter secret"
                        onChange={(e) => {
                          const nextValue = e.target.value;
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
                  ))}
                </div>
              ) : (
                <div className="integrations-admin-empty">No secrets declared.</div>
              )}
              <div className="integrations-admin-item-actions">
                <Button
                  onClick={async () => {
                    const valuesToSave = secretValues[selectedIntegration.id] || {};
                    const filtered = Object.fromEntries(
                      Object.entries(valuesToSave).filter(([, v]) => String(v || '').trim() !== '')
                    );
                    if (!Object.keys(filtered).length) return;
                    setSaving((prev) => ({ ...prev, [selectedIntegration.id]: true }));
                    const result = await setIntegrationSecrets(selectedIntegration.id, filtered);
                    if (!result.success) {
                      setError(result.error || 'Failed to save secrets');
                    } else {
                      setError('');
                      setSecretValues((prev) => ({ ...prev, [selectedIntegration.id]: {} }));
                    }
                    setSaving((prev) => ({ ...prev, [selectedIntegration.id]: false }));
                  }}
                  disabled={saving[selectedIntegration.id]}
                >
                  {saving[selectedIntegration.id] ? 'Saving…' : 'Save secrets'}
                </Button>
              </div>
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
                        <Button variant="secondary" disabled>
                          <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                          <span className="btn-label">Remove</span>
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
                  <Button variant="secondary" disabled>
                    <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                    <span className="btn-label">Remove</span>
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
          <div className="integrations-admin-card-subtitle">Curated integrations will appear here soon.</div>
          <div className="integrations-admin-marketplace-placeholder">
            <FontAwesomeIcon icon={faStore} className="integrations-admin-marketplace-icon" />
            <div className="integrations-admin-marketplace-badge">Under development</div>
            <div className="integrations-admin-marketplace-title">Marketplace is on the way</div>
            <div className="integrations-admin-marketplace-text">
              We’re preparing a curated catalog of integrations with verified compatibility, ratings, and guided setup.
            </div>
          </div>
        </GlassCard>
      )}

      {modal}
    </div>
  );
}
