import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faCheckCircle,
  faCubes,
  faDownload,
  faImages,
  faLink,
  faPlug,
  faStar,
  faUsers,
} from '@fortawesome/free-solid-svg-icons';
import Button from '../../common/Button/Button';
import IntegrationIcon from '../../common/IntegrationIcon/IntegrationIcon';
import GalleryCarousel from '../../common/GalleryCarousel/GalleryCarousel';
import BaseModal from '../../common/BaseModal/BaseModal';

export default function MarketplaceIntegrationModal({
  integration,
  onClose,
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
  if (!integration) return null;

  const iconRaw = integration.assets?.icon || integration.icon || '';
  const fa = resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug;
  const versionLabel = getMarketplaceVersion(integration);
  const isInstalled = Boolean(integration.installed || installedIds.has(integration.id));
  const downloads = formatDownloads ? formatDownloads(integration.downloads) : (integration.downloads || 0);
  const galleryImages = integration.images;

  return (
    <BaseModal
      open
      onClose={onClose}
      dialogClassName="integrations-admin-modal"
      closeAriaLabel="Close integration dialog"
      onBackdropMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div className="auth-modal-content">
          <div className="integrations-admin-modal-header">
            <div className="integrations-admin-modal-title">
              <div className="integrations-admin-modal-icon" aria-hidden="true">
                <IntegrationIcon
                  icon={iconRaw}
                  faIcon={fa}
                  fallbackIcon={faPlug}
                  onError={(e) => { e.currentTarget.style.display = 'none'; }}
                />
              </div>
              <div>
                <div className="integrations-admin-modal-eyebrow">Marketplace</div>
                <h3>{getMarketplaceName(integration)}</h3>
                <div className="integrations-admin-modal-sub">/{integration.id}</div>
              </div>
            </div>
            <div className="integrations-admin-modal-meta">
              <span className="integrations-admin-badge">v{versionLabel}</span>
              <span className="integrations-admin-badge"><FontAwesomeIcon icon={faDownload} /> {downloads}</span>
              {integration.featured ? (
                <span className="integrations-admin-badge"><FontAwesomeIcon icon={faStar} /> Featured</span>
              ) : null}
              {integration.verified ? (
                <span className="integrations-admin-badge"><FontAwesomeIcon icon={faCheckCircle} /> Verified</span>
              ) : (
                <span className="integrations-admin-badge"><FontAwesomeIcon icon={faUsers} /> Community</span>
              )}
            </div>
          </div>
          <div className="integrations-admin-modal-body">
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-modal-desc">
                {integration.description || 'No description provided yet.'}
              </div>
            </div>
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">
                <FontAwesomeIcon icon={faCubes} /> Details
              </div>
              <div className="integrations-admin-marketplace-details">
                <div><strong>Listen path:</strong> {integration.listen_path || `/integrations/${integration.id}`}</div>
                <div><strong>Release tag:</strong> {integration.release_tag || 'N/A'}</div>
                {integration.homepage ? (
                  <div><strong>Homepage:</strong> <a href={integration.homepage} target="_blank" rel="noreferrer">{integration.homepage}</a></div>
                ) : null}
              </div>
            </div>
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">
                <FontAwesomeIcon icon={faLink} /> Links
              </div>
              <div className="integrations-admin-marketplace-links">
                {integration.repo_url ? (
                  <a className="integrations-admin-link-pill" href={integration.repo_url} target="_blank" rel="noreferrer">Repository</a>
                ) : null}
                {integration.manifest_url ? (
                  <a className="integrations-admin-link-pill" href={integration.manifest_url} target="_blank" rel="noreferrer">Manifest</a>
                ) : null}
              </div>
            </div>
            {Array.isArray(galleryImages) && galleryImages.length ? (
              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-card-title">
                  <FontAwesomeIcon icon={faImages} /> Gallery
                </div>
                <GalleryCarousel images={galleryImages} />
              </div>
            ) : null}
            <div className="integrations-admin-modal-section">
              <div className="integrations-admin-card-title">Install</div>
              <div className="integrations-admin-item-actions">
                <Button
                  variant={isInstalled ? 'secondary' : 'primary'}
                  onClick={() => onInstallIntegration(integration)}
                  disabled={isInstalled || installing[integration.id]}
                >
                  {isInstalled ? 'Installed' : (installing[integration.id] ? 'Installing…' : 'Install')}
                </Button>
              </div>
              {installing[integration.id] ? (
                <div className="integrations-admin-install-status">
                  <div className="integrations-admin-install-meta">
                    <span>{installStatus[integration.id]?.message || 'Installing…'}</span>
                    <span>{installStatus[integration.id]?.progress ?? 0}%</span>
                  </div>
                  <div className="integrations-admin-install-bar">
                    <span style={{ width: `${installStatus[integration.id]?.progress ?? 10}%` }} />
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </div>
    </BaseModal>
  );
}
