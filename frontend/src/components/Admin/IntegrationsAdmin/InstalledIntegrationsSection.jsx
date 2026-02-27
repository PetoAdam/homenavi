import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowsRotate,
  faCubes,
  faDownload,
  faKey,
  faMagnifyingGlass,
  faPlug,
  faPowerOff,
  faPuzzlePiece,
  faRoute,
} from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../../common/GlassCard/GlassCard';
import Toolbar from '../../common/Toolbar/Toolbar';
import Button from '../../common/Button/Button';
import RoleSelect from '../../common/RoleSelect/RoleSelect';
import SearchBar from '../../common/SearchBar/SearchBar';
import { hasSetupUiPath } from '../../../utils/integrationSetup';
import IntegrationCard, { IntegrationCardHeader } from '../../common/IntegrationCard/IntegrationCard';
import IntegrationIcon from '../../common/IntegrationIcon/IntegrationIcon';
import GlassSwitch from '../../common/GlassSwitch/GlassSwitch';
import '../../common/Toolbar/Toolbar.css';

export default function InstalledIntegrationsSection({
  integrations,
  page,
  totalPages,
  pageSize,
  pageSizeOptions,
  query,
  onQueryChange,
  onPageChange,
  onPageSizeChange,
  onSearchSubmit,
  onReload,
  reloading,
  onRestartAll,
  restartingAll,
  onOpenIntegration,
  onOpenManage,
  onRestartIntegration,
  onUninstallIntegration,
  onUpdateIntegration,
  onToggleAutoUpdate,
  restarting,
  uninstalling,
  updating,
  installStatus,
  setupCapabilities,
  onOpenSetup,
  resolveFaIcon,
}) {
  return (
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
                <Button
                  variant="secondary"
                  className="hn-toolbar-iconbtn"
                  onClick={onReload}
                  disabled={reloading}
                  aria-label="Refresh integrations"
                >
                  <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
                </Button>
              </div>
              <div className="hn-toolbar-group">
                <Button
                  variant="secondary"
                  className="hn-toolbar-iconbtn"
                  onClick={onRestartAll}
                  disabled={restartingAll}
                  aria-label="Restart all integrations"
                >
                  <span className="btn-icon"><FontAwesomeIcon icon={faPowerOff} /></span>
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
          onSearchSubmit();
        }}
      >
        <SearchBar
          value={query}
          onChange={onQueryChange}
          onClear={onSearchSubmit}
          placeholder="Search integrations"
          ariaLabel="Search integrations"
          className="integrations-admin-searchbar"
        />
        <RoleSelect
          value={`${pageSize}/page`}
          options={pageSizeOptions.map((n) => `${n}/page`)}
          onChange={onPageSizeChange}
        />
        <Button type="submit">
          <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlass} /></span>
          <span className="btn-label">Search</span>
        </Button>
      </form>

      {integrations.length ? (
        <div className="integration-card-grid">
          {integrations.map((integration) => {
            const iconRaw = integration.icon || '';
            const fa = resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug;
            const widgetsCount = integration.widgets?.length || 0;
            const secretsCount = Array.isArray(integration.secrets) ? integration.secrets.length : 0;
            const route = integration.route || `/apps/${integration.id}`;
            const installedVersion = integration.installed_version || 'unknown';
            const latestVersion = integration.latest_version || '';
            const updateAvailable = Boolean(integration.update_available && latestVersion);
            const autoUpdate = Boolean(integration.auto_update);
            const updateBusy = Boolean(updating[integration.id] || integration.update_in_progress);
            const status = installStatus[integration.id];
            const setupCapable = hasSetupUiPath(integration);
            return (
              <IntegrationCard
                key={integration.id}
                onClick={() => onOpenIntegration(integration)}
                header={(
                  <IntegrationCardHeader
                    eyebrow="Installed"
                    title={integration.display_name || integration.id}
                    subtitle={`/${integration.id}`}
                    icon={(
                      <IntegrationIcon
                        icon={iconRaw}
                        faIcon={fa}
                        fallbackIcon={faPlug}
                      />
                    )}
                  />
                )}
                description={integration.description || integration.summary || 'No description provided yet.'}
                meta={(
                  <>
                    <div className="integration-card-meta">
                      <span className="integration-card-badge"><FontAwesomeIcon icon={faCubes} /> {widgetsCount} widgets</span>
                      <span className="integration-card-badge"><FontAwesomeIcon icon={faKey} /> {secretsCount} secrets</span>
                    </div>
                    <div className="integration-card-meta">
                      <span className="integration-card-badge"><FontAwesomeIcon icon={faRoute} /> {route}</span>
                    </div>
                    <div className="integration-card-meta integrations-admin-update-meta">
                      <span className="integration-card-badge">Installed: {installedVersion}</span>
                      {latestVersion ? <span className="integration-card-badge">Latest: {latestVersion}</span> : null}
                      {updateAvailable ? <span className="integration-card-badge integrations-admin-update-badge">Update available</span> : null}
                    </div>
                  </>
                )}
                actions={(
                  <div className="integrations-admin-card-actions-layout">
                    <div className="integrations-admin-card-actions-main">
                      <Button
                        className="integration-card-action-btn integrations-admin-action-primary"
                        onClick={() => onUpdateIntegration(integration.id)}
                        disabled={!updateAvailable || updateBusy}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faDownload} /></span>
                        <span className="btn-label">{updateBusy ? 'Updating…' : 'Update'}</span>
                      </Button>
                      <Button
                        variant="secondary"
                        className="integration-card-action-btn"
                        onClick={() => onOpenManage(integration)}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faCubes} /></span>
                        <span className="btn-label">Manage</span>
                      </Button>
                      {setupCapable ? (
                        <Button
                          variant="secondary"
                          className="integration-card-action-btn"
                          onClick={() => onOpenSetup(integration)}
                        >
                          <span className="btn-icon"><FontAwesomeIcon icon={faRoute} /></span>
                          <span className="btn-label">Open setup</span>
                        </Button>
                      ) : null}
                    </div>
                    <div className="integrations-admin-card-actions-sub">
                      <Button
                        variant="ghost"
                        className="integration-card-action-btn"
                        onClick={() => onRestartIntegration(integration.id)}
                        disabled={restarting[integration.id]}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
                        <span className="btn-label">{restarting[integration.id] ? 'Restarting…' : 'Restart'}</span>
                      </Button>
                      <Button
                        variant="ghost"
                        className="integration-card-action-btn integrations-admin-action-danger"
                        onClick={() => onUninstallIntegration(integration.id)}
                        disabled={uninstalling[integration.id]}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                        <span className="btn-label">{uninstalling[integration.id] ? 'Removing…' : 'Remove'}</span>
                      </Button>
                    </div>
                    <div className="integrations-admin-autoupdate-toggle" onClick={(e) => e.stopPropagation()}>
                      <GlassSwitch
                        checked={autoUpdate}
                        disabled={updateBusy}
                        onChange={(next) => onToggleAutoUpdate(integration.id, next)}
                      />
                      <span>Auto-update</span>
                    </div>
                  </div>
                )}
                footer={updateBusy ? (
                  <div className="integrations-admin-install-status" onClick={(e) => e.stopPropagation()}>
                    <div className="integrations-admin-install-meta">
                      <span>{status?.message || 'Updating…'}</span>
                      <span>{status?.progress ?? 0}%</span>
                    </div>
                    <div className="integrations-admin-install-bar">
                      <span style={{ width: `${status?.progress ?? 10}%` }} />
                    </div>
                  </div>
                ) : null}
              />
            );
          })}
        </div>
      ) : (
        <div className="integrations-admin-empty">No integrations found.</div>
      )}

      <div className="integrations-admin-pagination">
        <Button variant="secondary" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>Prev</Button>
        <span className="integrations-admin-muted">Page {page} of {totalPages}</span>
        <Button variant="secondary" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>Next</Button>
      </div>
    </GlassCard>
  );
}
