import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowsRotate,
  faCubes,
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
import IntegrationCard, { IntegrationCardHeader } from '../../common/IntegrationCard/IntegrationCard';
import IntegrationIcon from '../../common/IntegrationIcon/IntegrationIcon';
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
  restarting,
  uninstalling,
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
        <input
          className="input integrations-admin-input"
          placeholder="Search integrations"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
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
                  </>
                )}
                actions={(
                  <>
                    <Button
                      variant="secondary"
                      className="integration-card-action-btn"
                      onClick={() => onOpenManage(integration)}
                    >
                      <span className="btn-icon"><FontAwesomeIcon icon={faCubes} /></span>
                      <span className="btn-label">Manage</span>
                    </Button>
                    <Button
                      variant="secondary"
                      className="integration-card-action-btn"
                      onClick={() => onRestartIntegration(integration.id)}
                      disabled={restarting[integration.id]}
                    >
                      <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
                      <span className="btn-label">{restarting[integration.id] ? 'Restarting…' : 'Restart'}</span>
                    </Button>
                    <Button
                      variant="secondary"
                      className="integration-card-action-btn"
                      onClick={() => onUninstallIntegration(integration.id)}
                      disabled={uninstalling[integration.id]}
                    >
                      <span className="btn-icon"><FontAwesomeIcon icon={faPlug} /></span>
                      <span className="btn-label">{uninstalling[integration.id] ? 'Removing…' : 'Remove'}</span>
                    </Button>
                  </>
                )}
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
