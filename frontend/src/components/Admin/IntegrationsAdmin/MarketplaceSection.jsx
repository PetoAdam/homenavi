import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowDown,
  faCheckCircle,
  faCompass,
  faDownload,
  faFire,
  faStar,
  faStore,
  faUsers,
  faPlug,
} from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../../common/GlassCard/GlassCard';
import Button from '../../common/Button/Button';
import RoleSelect from '../../common/RoleSelect/RoleSelect';
import IntegrationCard, { IntegrationCardHeader } from '../../common/IntegrationCard/IntegrationCard';
import IntegrationIcon from '../../common/IntegrationIcon/IntegrationIcon';
import SearchBar from '../../common/SearchBar/SearchBar';

export default function MarketplaceSection({
  marketplaceError,
  marketplaceLoading,
  featuredMarketplace,
  filteredMarketplace,
  marketplaceMode,
  marketplaceFilter,
  marketplaceSort,
  marketplaceQuery,
  marketplaceShowInstalled,
  onModeChange,
  onFilterChange,
  onSortChange,
  onQueryChange,
  onShowInstalledChange,
  onSelectIntegration,
  onInstallIntegration,
  installing,
  installStatus,
  installedIds,
  resolveFaIcon,
  getMarketplaceName,
  getMarketplacePublisher,
  getMarketplaceVersion,
  formatDownloads,
}) {
  const sortOptions = ['Name', 'Version', 'Downloads', 'Trending'];
  const sortLabel = (() => {
    if (marketplaceSort === 'version') return 'Version';
    if (marketplaceSort === 'downloads') return 'Downloads';
    if (marketplaceSort === 'trending') return 'Trending';
    return 'Name';
  })();

  return (
    <GlassCard className="integrations-admin-card" interactive={false}>
      <div className="integrations-admin-card-title">
        <FontAwesomeIcon icon={faStore} />
        Marketplace
      </div>
      {marketplaceError ? <div className="integrations-admin-error">{marketplaceError}</div> : null}

      <div className="integrations-marketplace-nav">
        {["discover", "trending", "downloads"].map((item) => (
          <button
            key={item}
            type="button"
            className={`integrations-marketplace-nav-btn${marketplaceMode === item ? ' active' : ''}`}
            onClick={() => onModeChange(item)}
          >
            <FontAwesomeIcon icon={item === 'discover' ? faCompass : item === 'trending' ? faFire : faArrowDown} />
            <span>{item === 'downloads' ? 'Downloads' : (item === 'discover' ? 'Discover' : 'Trending')}</span>
          </button>
        ))}
      </div>

      <div className="integrations-marketplace-toolbar">
        <SearchBar
          value={marketplaceQuery}
          onChange={onQueryChange}
          onClear={() => onQueryChange('')}
          placeholder="Search marketplace"
          ariaLabel="Search marketplace"
          className="integrations-admin-searchbar"
        />
        <div className="integrations-marketplace-filters">
          {["all", "featured", "verified", "community"].map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => onFilterChange(item)}
              className={`integrations-marketplace-filter${marketplaceFilter === item ? ' active' : ''}`}
            >
              <FontAwesomeIcon
                icon={item === 'all'
                  ? faStore
                  : item === 'featured'
                    ? faStar
                    : item === 'verified'
                      ? faCheckCircle
                      : faUsers}
              />
              <span>{item === 'all'
                ? 'All'
                : item === 'featured'
                  ? 'Featured'
                  : item === 'verified'
                    ? 'Verified'
                    : 'Community'}
              </span>
            </button>
          ))}
          <label className="integrations-marketplace-toggle">
            <input
              type="checkbox"
              checked={marketplaceShowInstalled}
              onChange={(e) => onShowInstalledChange(e.target.checked)}
            />
            <span>Show installed</span>
          </label>
        </div>
        <div className="integrations-marketplace-sort">
          <RoleSelect
            value={sortLabel}
            options={sortOptions}
            onChange={(value) => {
              const nextValue = value === 'Version'
                ? 'version'
                : value === 'Downloads'
                  ? 'downloads'
                  : value === 'Trending'
                    ? 'trending'
                    : 'name';
              onSortChange(nextValue);
            }}
          />
        </div>
      </div>

      <div className="integrations-admin-divider" />

      {marketplaceLoading ? (
        <div className="integrations-admin-empty">Loading marketplace…</div>
      ) : (
        <>
          {featuredMarketplace.length ? (
            <div className="integrations-marketplace-featured">
              <div className="integrations-marketplace-featured-header">
                <div>
                  <div className="integrations-marketplace-eyebrow">Featured</div>
                  <div className="integrations-marketplace-title">Spotlight integrations</div>
                </div>
                <Button variant="secondary" onClick={() => onFilterChange('featured')}>
                  View all
                </Button>
              </div>
              <div className="integration-card-grid">
                {featuredMarketplace.map((entry) => {
                  const iconRaw = entry.assets?.icon || entry.icon || '';
                  const fa = resolveFaIcon(iconRaw) || resolveFaIcon(entry.id) || faPlug;
                  const isInstalled = Boolean(entry.installed || installedIds.has(entry.id));
                  const versionLabel = getMarketplaceVersion(entry);
                  return (
                    <IntegrationCard
                      key={`${entry.id}-featured`}
                      onClick={() => onSelectIntegration(entry)}
                      header={(
                        <IntegrationCardHeader
                          eyebrow={getMarketplacePublisher(entry)}
                          title={getMarketplaceName(entry)}
                          version={versionLabel}
                          badges={entry.verified ? [
                            <>
                              <FontAwesomeIcon icon={faCheckCircle} /> Verified
                            </>
                          ] : []}
                          icon={(
                            <IntegrationIcon
                              icon={iconRaw}
                              faIcon={fa}
                              fallbackIcon={faPlug}
                            />
                          )}
                        />
                      )}
                      description={entry.description || 'No description provided yet.'}
                      meta={(
                        <>
                          <div className="integration-card-meta">
                            <span className="integration-card-badge">{entry.listen_path || `/integrations/${entry.id}`}</span>
                            {!entry.verified ? (
                              <span className="integration-card-badge"><FontAwesomeIcon icon={faUsers} /> Community</span>
                            ) : null}
                          </div>
                          <div className="integration-card-meta">
                            <span className="integration-card-badge"><FontAwesomeIcon icon={faDownload} /> {formatDownloads ? formatDownloads(entry.downloads) : (entry.downloads || 0)}</span>
                            {entry.featured ? (
                              <span className="integration-card-badge"><FontAwesomeIcon icon={faStar} /> Featured</span>
                            ) : null}
                          </div>
                        </>
                      )}
                      actions={(
                        <Button
                          variant={isInstalled ? 'secondary' : 'primary'}
                          className="integration-card-action-btn"
                          onClick={() => onInstallIntegration(entry)}
                          disabled={isInstalled || installing[entry.id]}
                        >
                          <span className="btn-icon"><FontAwesomeIcon icon={faDownload} /></span>
                          <span className="btn-label">
                            {isInstalled ? 'Installed' : (installing[entry.id] ? 'Installing…' : 'Install')}
                          </span>
                        </Button>
                      )}
                    />
                  );
                })}
              </div>
            </div>
          ) : null}

          {filteredMarketplace.length ? (
            <div className="integration-card-grid">
              {filteredMarketplace.map((entry) => {
                const iconRaw = entry.assets?.icon || entry.icon || '';
                const fa = resolveFaIcon(iconRaw) || resolveFaIcon(entry.id) || faPlug;
                const isInstalled = Boolean(entry.installed || installedIds.has(entry.id));
                const status = installStatus[entry.id];
                const versionLabel = getMarketplaceVersion(entry);
                return (
                  <IntegrationCard
                    key={entry.id}
                    onClick={() => onSelectIntegration(entry)}
                    header={(
                      <IntegrationCardHeader
                        eyebrow={getMarketplacePublisher(entry)}
                        title={getMarketplaceName(entry)}
                        version={versionLabel}
                        badges={entry.verified ? [
                          <>
                            <FontAwesomeIcon icon={faCheckCircle} /> Verified
                          </>
                        ] : []}
                        icon={(
                          <IntegrationIcon
                            icon={iconRaw}
                            faIcon={fa}
                            fallbackIcon={faPlug}
                          />
                        )}
                      />
                    )}
                    description={entry.description || 'No description provided yet.'}
                    meta={(
                      <>
                        <div className="integration-card-meta">
                          <span className="integration-card-badge">{entry.listen_path || `/integrations/${entry.id}`}</span>
                          {!entry.verified ? (
                            <span className="integration-card-badge"><FontAwesomeIcon icon={faUsers} /> Community</span>
                          ) : null}
                        </div>
                        <div className="integration-card-meta">
                          <span className="integration-card-badge"><FontAwesomeIcon icon={faDownload} /> {formatDownloads ? formatDownloads(entry.downloads) : (entry.downloads || 0)}</span>
                          {entry.featured ? (
                            <span className="integration-card-badge"><FontAwesomeIcon icon={faStar} /> Featured</span>
                          ) : null}
                        </div>
                      </>
                    )}
                    actions={(
                      <Button
                        variant={isInstalled ? 'secondary' : 'primary'}
                        className="integration-card-action-btn"
                        onClick={() => onInstallIntegration(entry)}
                        disabled={isInstalled || installing[entry.id]}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faDownload} /></span>
                        <span className="btn-label">
                          {isInstalled ? 'Installed' : (installing[entry.id] ? 'Installing…' : 'Install')}
                        </span>
                      </Button>
                    )}
                    footer={installing[entry.id] ? (
                      <div className="integrations-admin-install-status" onClick={(e) => e.stopPropagation()}>
                        <div className="integrations-admin-install-meta">
                          <span>{status?.message || 'Installing…'}</span>
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
            <div className="integrations-admin-empty">No marketplace integrations available.</div>
          )}
        </>
      )}
    </GlassCard>
  );
}
