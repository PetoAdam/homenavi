import React, { useEffect, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCubes, faEye, faKey, faPlug, faRoute, faStore } from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import PageHeader from '../common/PageHeader/PageHeader';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import GlassCard from '../common/GlassCard/GlassCard';
import SearchBar from '../common/SearchBar/SearchBar';
import IntegrationCard, { IntegrationCardHeader } from '../common/IntegrationCard/IntegrationCard';
import IntegrationIcon from '../common/IntegrationIcon/IntegrationIcon';
import { useAuth } from '../../context/AuthContext';
import { useIntegrationMarketplaceQuery, useIntegrationRegistryQuery } from '../../features/integrations/hooks/useIntegrationQueries';
import './IntegrationsAdmin.css';

const FA_ICON_MAP = {
  spotify: faSpotify,
  plug: faPlug,
};

function normalizeIconKey(iconName) {
  const raw = (iconName || '').toString().trim();
  if (!raw) return '';
  return raw.toLowerCase();
}

function resolveFaIcon(iconName) {
  const key = normalizeIconKey(iconName);
  if (!key) return null;
  const faKey = key.startsWith('fa:') ? key.slice('fa:'.length).trim() : key;
  return FA_ICON_MAP[faKey] || null;
}

function formatDownloads(value) {
  const count = Number(value || 0);
  if (!Number.isFinite(count)) return '0';
  if (count < 1000) return String(count);
  if (count < 1000000) return `${(count / 1000).toFixed(1).replace('.', ',')}k`;
  return `${(count / 1000000).toFixed(1).replace('.', ',')}M`;
}

function filterByQuery(items, query, fields) {
  const term = String(query || '').trim().toLowerCase();
  if (!term) return items;
  return items.filter((item) => fields.some((field) => String(item?.[field] || '').toLowerCase().includes(term)));
}

export default function IntegrationsAdminReadonly() {
  const { user, accessToken } = useAuth();
  const canView = user?.role === 'admin' || user?.role === 'resident';
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');

  useEffect(() => {
    const handle = setTimeout(() => setDebouncedQuery(query), query ? 250 : 0);
    return () => clearTimeout(handle);
  }, [query]);

  const registryQuery = useIntegrationRegistryQuery(
    { q: debouncedQuery, page: 1, pageSize: 100 },
    { enabled: Boolean(accessToken && canView) }
  );
  const marketplaceQuery = useIntegrationMarketplaceQuery({
    enabled: Boolean(accessToken && canView),
  });

  const marketplaceById = useMemo(() => {
    const map = new Map();
    (marketplaceQuery.data?.integrations || []).forEach((entry) => {
      if (entry?.id) map.set(entry.id, entry);
    });
    return map;
  }, [marketplaceQuery.data]);

  const installedIntegrations = useMemo(() => {
    const items = registryQuery.data?.integrations || [];
    return items.map((integration) => {
      const market = marketplaceById.get(integration.id);
      return {
        ...integration,
        description: market?.description || integration.description,
        publisher: market?.publisher || integration.publisher,
        downloads: market?.downloads,
      };
    });
  }, [marketplaceById, registryQuery.data]);

  const marketplaceIntegrations = useMemo(() => {
    const installedIds = new Set(installedIntegrations.map((entry) => entry.id));
    const items = marketplaceQuery.data?.integrations || [];
    return filterByQuery(items, query, ['name', 'display_name', 'id', 'publisher']).map((entry) => ({
      ...entry,
      installed: Boolean(entry.installed || installedIds.has(entry.id)),
    }));
  }, [installedIntegrations, marketplaceQuery.data, query]);

  const filteredInstalled = useMemo(
    () => filterByQuery(installedIntegrations, query, ['display_name', 'id', 'description', 'publisher']),
    [installedIntegrations, query]
  );

  if (!canView) {
    return (
      <UnauthorizedView
        title="Sign in required"
        message="This read-only integrations panel is available to resident and admin accounts."
      />
    );
  }

  const loading = registryQuery.isLoading || marketplaceQuery.isLoading;
  const error = registryQuery.error?.message || marketplaceQuery.error?.message || '';
  const installedCount = Number(registryQuery.data?.total || installedIntegrations.length || 0);
  const marketplaceCount = Number(marketplaceQuery.data?.integrations?.length || 0);

  return (
    <div className="integrations-admin-page integrations-admin-page--readonly">
      <PageHeader
        title="Integrations View"
        subtitle="Read-only overview of installed integrations, versions, routes, and marketplace entries."
      />

      <GlassCard className="integrations-admin-card integrations-admin-card--stack" interactive={false}>
        <div className="integrations-admin-section-heading integrations-admin-readonly-hero">
          <div className="integrations-admin-section-title">
            <FontAwesomeIcon icon={faEye} />
            <span>Read-only mode</span>
          </div>
          <div className="integrations-admin-section-sub">
            View integration inventory and marketplace metadata without lifecycle or secret-management actions.
          </div>
        </div>
        <div className="integrations-admin-readonly-stats">
          <span className="integration-card-badge">Installed: {installedCount}</span>
          <span className="integration-card-badge">Marketplace: {marketplaceCount}</span>
          <span className="integration-card-badge">Searchable snapshot</span>
        </div>
        <SearchBar
          value={query}
          onChange={setQuery}
          onClear={() => setQuery('')}
          placeholder="Search integrations, publishers, or routes"
          ariaLabel="Search integrations read-only panel"
          className="integrations-admin-searchbar"
        />
      </GlassCard>

      {error ? (
        <GlassCard className="integrations-admin-card integrations-admin-card--stack" interactive={false}>
          <div className="integrations-admin-error">{error}</div>
        </GlassCard>
      ) : null}

      <GlassCard className="integrations-admin-card integrations-admin-card--stack" interactive={false}>
        <div className="integrations-admin-card-title">
          <FontAwesomeIcon icon={faPlug} />
          <span>Installed integrations</span>
        </div>
        <div className="integrations-admin-card-subtitle">Status, versions, routes, widgets, and secret requirements.</div>
        {loading ? <div className="integrations-admin-empty">Loading integrations…</div> : null}
        {!loading && filteredInstalled.length ? (
          <div className="integration-card-grid">
            {filteredInstalled.map((integration) => {
              const iconRaw = integration.icon || '';
              const widgetsCount = integration.widgets?.length || 0;
              const secretsCount = Array.isArray(integration.secrets) ? integration.secrets.length : 0;
              const route = integration.route || `/apps/${integration.id}`;
              const latestVersion = integration.latest_version || '';
              return (
                <IntegrationCard
                  key={integration.id}
                  header={(
                    <IntegrationCardHeader
                      eyebrow="Installed"
                      title={integration.display_name || integration.id}
                      subtitle={`/${integration.id}`}
                      version={integration.installed_version || 'unknown'}
                      badges={integration.update_available && latestVersion ? ['Update available'] : []}
                      icon={(
                        <IntegrationIcon
                          icon={iconRaw}
                          faIcon={resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug}
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
                      <div className="integration-card-meta">
                        {latestVersion ? <span className="integration-card-badge">Latest: {latestVersion}</span> : null}
                        {integration.publisher ? <span className="integration-card-badge">By {integration.publisher}</span> : null}
                        {typeof integration.downloads !== 'undefined' ? <span className="integration-card-badge">{formatDownloads(integration.downloads)} downloads</span> : null}
                      </div>
                    </>
                  )}
                  footer={<div className="integrations-admin-readonly-note">Read-only snapshot</div>}
                />
              );
            })}
          </div>
        ) : null}
        {!loading && !filteredInstalled.length ? <div className="integrations-admin-empty">No installed integrations match the current search.</div> : null}
      </GlassCard>

      <GlassCard className="integrations-admin-card integrations-admin-card--stack" interactive={false}>
        <div className="integrations-admin-card-title">
          <FontAwesomeIcon icon={faStore} />
          <span>Marketplace catalog</span>
        </div>
        <div className="integrations-admin-card-subtitle">Read-only view of available integrations and their marketplace metadata.</div>
        {loading ? <div className="integrations-admin-empty">Loading marketplace…</div> : null}
        {!loading && marketplaceIntegrations.length ? (
          <div className="integration-card-grid">
            {marketplaceIntegrations.map((integration) => {
              const iconRaw = integration.assets?.icon || integration.icon || '';
              return (
                <IntegrationCard
                  key={integration.id}
                  header={(
                    <IntegrationCardHeader
                      eyebrow={integration.installed ? 'Installed + marketplace' : 'Marketplace'}
                      title={integration.name || integration.display_name || integration.id}
                      subtitle={integration.publisher || integration.id}
                      version={integration.version || '1.0.0'}
                      badges={[
                        integration.verified ? 'Verified' : null,
                        integration.featured ? 'Featured' : null,
                      ].filter(Boolean)}
                      icon={(
                        <IntegrationIcon
                          icon={iconRaw}
                          faIcon={resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug}
                          fallbackIcon={faPlug}
                        />
                      )}
                    />
                  )}
                  description={integration.description || 'No description provided yet.'}
                  meta={(
                    <div className="integration-card-meta">
                      <span className="integration-card-badge">{formatDownloads(integration.downloads)} downloads</span>
                      {integration.category ? <span className="integration-card-badge">{integration.category}</span> : null}
                      {integration.installed ? <span className="integration-card-badge">Installed</span> : null}
                    </div>
                  )}
                  footer={<div className="integrations-admin-readonly-note">Catalog metadata only</div>}
                />
              );
            })}
          </div>
        ) : null}
        {!loading && !marketplaceIntegrations.length ? <div className="integrations-admin-empty">No marketplace integrations match the current search.</div> : null}
      </GlassCard>
    </div>
  );
}