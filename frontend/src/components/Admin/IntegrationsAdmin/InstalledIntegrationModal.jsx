import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faCubes,
  faImages,
  faKey,
  faLink,
  faPlug,
  faRoute,
} from '@fortawesome/free-solid-svg-icons';
import ModalTabs from '../../common/ModalTabs/ModalTabs';
import Button from '../../common/Button/Button';
import IntegrationIcon from '../../common/IntegrationIcon/IntegrationIcon';
import GalleryCarousel from '../../common/GalleryCarousel/GalleryCarousel';
import BaseModal from '../../common/BaseModal/BaseModal';

export default function InstalledIntegrationModal({
  integration,
  activeTab,
  onTabChange,
  onClose,
  onRestartIntegration,
  onUninstallIntegration,
  restarting,
  uninstalling,
  normalizeSecrets,
  pendingSecretsId,
  secretValidation,
  secretActionStatus,
  secretValues,
  onSecretChange,
  onSaveSecrets,
  saving,
  onSetupLater,
  resolveFaIcon,
}) {
  if (!integration) return null;

  const iconRaw = integration.icon || '';
  const fa = resolveFaIcon(iconRaw) || resolveFaIcon(integration.id) || faPlug;
  const widgetsCount = integration.widgets?.length || 0;
  const secretsCount = Array.isArray(integration.secrets) ? integration.secrets.length : 0;
  const route = integration.route || `/apps/${integration.id}`;
  const description = integration.marketplace?.description || integration.description || integration.summary || 'No description provided yet.';
  const galleryImages = integration.marketplace?.images || integration.images;
  const listenPath = integration.listen_path || integration.marketplace?.listen_path || `/integrations/${integration.id}`;
  const releaseTag = integration.release_tag || integration.marketplace?.release_tag;
  const homepage = integration.marketplace?.homepage || integration.homepage;
  const repoUrl = integration.marketplace?.repo_url || integration.repo_url;
  const manifestUrl = integration.marketplace?.manifest_url || integration.manifest_url;
  const hasGallery = Array.isArray(galleryImages) && galleryImages.length > 0;
  const hasLinks = Boolean(homepage || repoUrl || manifestUrl);
  const manageTabLabel = secretsCount > 0 ? `Manage (${secretsCount})` : 'Manage';

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
              <div className="integrations-admin-modal-eyebrow">Integration</div>
              <h3>{integration.display_name || integration.id}</h3>
              <div className="integrations-admin-modal-sub">/{integration.id}</div>
            </div>
          </div>
          <div className="integrations-admin-modal-meta">
            <span className="integrations-admin-badge"><FontAwesomeIcon icon={faCubes} /> {widgetsCount} widgets</span>
            <span className="integrations-admin-badge"><FontAwesomeIcon icon={faKey} /> {secretsCount} secrets</span>
            <span className="integrations-admin-badge"><FontAwesomeIcon icon={faRoute} /> {route}</span>
          </div>
        </div>

        <ModalTabs
          tabs={[
            { id: 'about', label: 'About' },
            { id: 'manage', label: manageTabLabel },
          ]}
          activeTab={activeTab}
          onChange={onTabChange}
        />

        <div className="integrations-admin-modal-body">
          {activeTab === 'about' ? (
            <>
              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-modal-desc">
                  {description}
                </div>
              </div>

              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-card-title">
                  <FontAwesomeIcon icon={faCubes} /> Details
                </div>
                <div className="integrations-admin-marketplace-details">
                  <div><strong>Listen path:</strong> {listenPath}</div>
                  <div><strong>Route:</strong> {route}</div>
                  {releaseTag ? <div><strong>Release tag:</strong> {releaseTag}</div> : null}
                </div>
              </div>

              {hasLinks ? (
                <div className="integrations-admin-modal-section">
                  <div className="integrations-admin-card-title">
                    <FontAwesomeIcon icon={faLink} /> Links
                  </div>
                  <div className="integrations-admin-marketplace-links">
                    {repoUrl ? (
                      <a className="integrations-admin-link-pill" href={repoUrl} target="_blank" rel="noreferrer">Repository</a>
                    ) : null}
                    {manifestUrl ? (
                      <a className="integrations-admin-link-pill" href={manifestUrl} target="_blank" rel="noreferrer">Manifest</a>
                    ) : null}
                    {homepage ? (
                      <a className="integrations-admin-link-pill" href={homepage} target="_blank" rel="noreferrer">Homepage</a>
                    ) : null}
                  </div>
                </div>
              ) : null}

              {hasGallery ? (
                <div className="integrations-admin-modal-section">
                  <div className="integrations-admin-card-title">
                    <FontAwesomeIcon icon={faImages} /> Gallery
                  </div>
                  <GalleryCarousel images={galleryImages} />
                </div>
              ) : null}

              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-card-title">Manage</div>
                <div className="integrations-admin-item-actions">
                  <Button
                    variant="secondary"
                    onClick={() => onRestartIntegration(integration.id)}
                    disabled={restarting[integration.id]}
                  >
                    {restarting[integration.id] ? 'Restarting…' : 'Restart integration'}
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => onUninstallIntegration(integration.id)}
                    disabled={uninstalling[integration.id]}
                  >
                    {uninstalling[integration.id] ? 'Removing…' : 'Remove'}
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <>
              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-card-title">Manage</div>
                <div className="integrations-admin-item-actions integrations-admin-secret-actions">
                  <Button
                    variant="secondary"
                    onClick={() => onRestartIntegration(integration.id)}
                    disabled={restarting[integration.id]}
                  >
                    {restarting[integration.id] ? 'Restarting…' : 'Restart integration'}
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={() => onUninstallIntegration(integration.id)}
                    disabled={uninstalling[integration.id]}
                  >
                    {uninstalling[integration.id] ? 'Removing…' : 'Remove'}
                  </Button>
                </div>
              </div>

              <div className="integrations-admin-modal-section">
                <div className="integrations-admin-card-title">Secrets</div>
                <div className="integrations-admin-card-subtitle">Write-only fields. Values are never read back.</div>
                {pendingSecretsId === integration.id ? (
                  <div className="integrations-admin-secret-callout">
                    This integration requires secrets. Please add them now to finish setup.
                  </div>
                ) : null}
                {secretValidation[integration.id]?.message ? (
                  <div className="integrations-admin-secret-error">
                    {secretValidation[integration.id].message}
                  </div>
                ) : null}
                {secretActionStatus[integration.id] ? (
                  <div className="integrations-admin-install-status">
                    <div className="integrations-admin-install-meta">
                      <span>{secretActionStatus[integration.id].message}</span>
                      <span>{secretActionStatus[integration.id].progress}%</span>
                    </div>
                    <div className="integrations-admin-install-bar">
                      <span style={{ width: `${secretActionStatus[integration.id].progress}%` }} />
                    </div>
                  </div>
                ) : null}
                {normalizeSecrets(integration.secrets).length ? (
                  <div className="integrations-admin-secret-list">
                    {normalizeSecrets(integration.secrets).map((spec) => {
                      const missing = secretValidation[integration.id]?.missing?.includes(spec.key);
                      const errorNonce = secretValidation[integration.id]?.nonce || 0;
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
                            value={(secretValues[integration.id] || {})[spec.key] || ''}
                            placeholder="Enter secret"
                            onChange={(e) => onSecretChange(integration.id, spec.key, e.target.value)}
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
                    onClick={() => onSaveSecrets(integration.id)}
                    disabled={saving[integration.id]}
                  >
                    {saving[integration.id] ? 'Saving…' : 'Save & restart'}
                  </Button>
                  {normalizeSecrets(integration.secrets).length ? (
                    <Button variant="secondary" onClick={onSetupLater}>
                      Set up later
                    </Button>
                  ) : null}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </BaseModal>
  );
}