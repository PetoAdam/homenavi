import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowDown,
  faCircleInfo,
  faClockRotateLeft,
  faLayerGroup,
  faSliders,
  faWandMagicSparkles,
} from '@fortawesome/free-solid-svg-icons';
import BaseModal from '../../common/BaseModal/BaseModal';

const MODE_LABELS = {
  '4': '4 columns',
  '3': '3 columns',
  '2': '2 columns',
  '1': '1 column',
};

export default function DashboardLayoutToolsModal({
  open,
  onClose,
  currentMode,
  importSourceMode,
  onImportSourceModeChange,
  onImportIntoCurrent,
  autoLayoutReorganize,
  autoLayoutGrowWidths,
  autoLayoutShrinkWidths,
  autoLayoutGrowHeights,
  autoLayoutShrinkHeights,
  onAutoLayoutOptionChange,
  onAutoLayoutCurrent,
}) {
  const importOptions = ['4', '3', '2', '1'].filter((mode) => mode !== currentMode);

  return (
    <BaseModal
      open={open}
      onClose={onClose}
      backdropClassName="widget-settings__backdrop"
      dialogClassName="widget-settings-modal dashboard-tools-modal"
      closeAriaLabel="Close layout tools"
    >
      <div className="widget-settings__header dashboard-tools-modal__header">
        <div className="widget-settings__icon dashboard-tools-modal__hero-icon">
          <FontAwesomeIcon icon={faSliders} />
        </div>
        <div className="widget-settings__header-text">
          <h2 className="widget-settings__title">Layout Tools</h2>
          <span className="widget-settings__type">Reorganize the current dashboard while keeping the editor consistent with the rest of Homenavi.</span>
        </div>
      </div>

      <div className="widget-settings__content dashboard-tools-modal__content">
        <section className="dashboard-tools-modal__section">
          <div className="dashboard-tools-modal__section-head">
            <FontAwesomeIcon icon={faLayerGroup} className="dashboard-tools-modal__section-icon" />
            <h3 className="dashboard-tools-modal__section-title">Current mode</h3>
          </div>
          <p className="dashboard-tools-modal__text">Editing {MODE_LABELS[currentMode] || currentMode}.</p>
          <div className="dashboard-tools-modal__info-pill">
            <FontAwesomeIcon icon={faClockRotateLeft} />
            <span>Every tool action is designed to be safely reversible with undo and redo.</span>
          </div>
        </section>

        <section className="dashboard-tools-modal__section">
          <div className="dashboard-tools-modal__section-head">
            <FontAwesomeIcon icon={faArrowDown} className="dashboard-tools-modal__section-icon" />
            <h3 className="dashboard-tools-modal__section-title">Import</h3>
          </div>
          <p className="dashboard-tools-modal__text">Copy the ordering from another column mode into the current one.</p>
          <div className="dashboard-tools-modal__row">
            <label className="dashboard-tools-modal__label" htmlFor="dashboard-tools-import-source">Source layout</label>
            <select
              id="dashboard-tools-import-source"
              className="widget-settings__select dashboard-tools-modal__select"
              value={importSourceMode}
              onChange={(event) => onImportSourceModeChange(event.target.value)}
            >
              {importOptions.map((mode) => (
                <option key={mode} value={mode}>{MODE_LABELS[mode]}</option>
              ))}
            </select>
          </div>
          <div className="dashboard-tools-modal__actions">
            <button className="widget-settings__btn widget-settings__btn--save dashboard-tools-modal__button" onClick={onImportIntoCurrent}>
              <FontAwesomeIcon icon={faArrowDown} />
              <span>Import into current</span>
            </button>
          </div>
        </section>

        <section className="dashboard-tools-modal__section">
          <div className="dashboard-tools-modal__section-head">
            <FontAwesomeIcon icon={faWandMagicSparkles} className="dashboard-tools-modal__section-icon" />
            <h3 className="dashboard-tools-modal__section-title">Auto-layout</h3>
          </div>
          <p className="dashboard-tools-modal__text">Use these options to choose whether auto-layout should move widgets, make them wider or narrower, and make them taller or shorter.</p>
          <div className="widget-settings__checkboxes dashboard-tools-modal__checkboxes">
            <label className="widget-settings__checkbox dashboard-tools-modal__checkbox">
              <input
                type="checkbox"
                checked={autoLayoutReorganize}
                onChange={(event) => onAutoLayoutOptionChange('reorganize', event.target.checked)}
              />
              <span>Move widgets into a tighter order</span>
            </label>
            <label className="widget-settings__checkbox dashboard-tools-modal__checkbox">
              <input
                type="checkbox"
                checked={autoLayoutGrowWidths}
                onChange={(event) => onAutoLayoutOptionChange('growWidths', event.target.checked)}
              />
              <span>Make widgets wider when there is empty space beside them</span>
            </label>
            <label className="widget-settings__checkbox dashboard-tools-modal__checkbox">
              <input
                type="checkbox"
                checked={autoLayoutShrinkWidths}
                onChange={(event) => onAutoLayoutOptionChange('shrinkWidths', event.target.checked)}
              />
              <span>Make widgets narrower before arranging them</span>
            </label>
            <label className="widget-settings__checkbox dashboard-tools-modal__checkbox">
              <input
                type="checkbox"
                checked={autoLayoutGrowHeights}
                onChange={(event) => onAutoLayoutOptionChange('growHeights', event.target.checked)}
              />
              <span>Make widgets taller when there is empty space below them</span>
            </label>
            <label className="widget-settings__checkbox dashboard-tools-modal__checkbox">
              <input
                type="checkbox"
                checked={autoLayoutShrinkHeights}
                onChange={(event) => onAutoLayoutOptionChange('shrinkHeights', event.target.checked)}
              />
              <span>Make widgets shorter before arranging them</span>
            </label>
          </div>
          <div className="dashboard-tools-modal__actions">
            <button className="widget-settings__btn widget-settings__btn--save dashboard-tools-modal__button" onClick={onAutoLayoutCurrent}>
              <FontAwesomeIcon icon={faWandMagicSparkles} />
              <span>Auto-layout current</span>
            </button>
          </div>
        </section>

        <section className="dashboard-tools-modal__section dashboard-tools-modal__section--hint">
          <div className="dashboard-tools-modal__section-head">
            <FontAwesomeIcon icon={faCircleInfo} className="dashboard-tools-modal__section-icon" />
            <h3 className="dashboard-tools-modal__section-title">Tip</h3>
          </div>
          <p className="dashboard-tools-modal__text">Import is best when another saved mode already looks right. Auto-layout is best when you want to repack the current mode and then fine-tune widths and heights without endless growth.</p>
        </section>
      </div>
    </BaseModal>
  );
}