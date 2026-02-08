import React, { useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faPlus,
  faSun,
  faMap,
  faLightbulb,
  faChartLine,
  faBolt,
  faPlug,
  faStar,
  faMusic,
  faCheck,
  faQuestionCircle,
} from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import { getModalRoot } from '../../common/Modal/modalRoot';
import SearchBar from '../../common/SearchBar/SearchBar';
import './AddWidgetModal.css';

// Icon mapping for widget types
const WIDGET_ICONS = {
  'homenavi.weather': faSun,
  'homenavi.map': faMap,
  'homenavi.device': faLightbulb,
  'homenavi.device.graph': faChartLine,
  'homenavi.automation.manual_trigger': faBolt,
};

const ICONS_BY_NAME = {
  sun: faSun,
  map: faMap,
  lightbulb: faLightbulb,
  chart: faChartLine,
  bolt: faBolt,
  plug: faPlug,
  sparkles: faStar,
  music: faMusic,
  spotify: faSpotify,
};

function getWidgetIcon(widget) {
  const byID = WIDGET_ICONS[widget?.id];
  const raw = (widget?.icon || '').trim();

  if (raw.startsWith('/') || raw.startsWith('http://') || raw.startsWith('https://')) {
    return { type: 'image', value: raw };
  }

  const key = raw.toLowerCase();
  return { type: 'fa', value: ICONS_BY_NAME[key] || byID || faQuestionCircle };
}

export default function AddWidgetModal({ open, onClose, catalog, onAdd }) {
  const [search, setSearch] = useState('');

  // Filter catalog by search
  const filteredCatalog = useMemo(() => {
    if (!search.trim()) return catalog || [];

    const q = search.toLowerCase().trim();
    return (catalog || []).filter((widget) => {
      const name = (widget.display_name || widget.id || '').toLowerCase();
      const desc = (widget.description || '').toLowerCase();
      return name.includes(q) || desc.includes(q);
    });
  }, [catalog, search]);

  // Close on backdrop click
  const handleBackdropClick = (e) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  // Handle add
  const handleAdd = (widgetType) => {
    onAdd(widgetType, {});
  };

  if (!open) return null;

  return createPortal(
    <div className="widget-settings__backdrop open add-widget-modal__backdrop" onClick={handleBackdropClick}>
      <div className="widget-settings-modal add-widget-modal">
        <button
          type="button"
          className="widget-settings__close"
          onClick={onClose}
          aria-label="Close"
        >
          &times;
        </button>

        <div className="widget-settings__header">
          <div className="widget-settings__icon">
            <FontAwesomeIcon icon={faPlus} />
          </div>
          <div className="widget-settings__header-text">
            <h2 className="widget-settings__title">Add Widget</h2>
            <span className="widget-settings__type">Choose from the catalog</span>
          </div>
        </div>

        <div className="add-widget-modal__content">
          <div className="add-widget-modal__search">
            <SearchBar
              value={search}
              onChange={setSearch}
              onClear={() => setSearch('')}
              placeholder="Search widgets…"
              autoFocus
              ariaLabel="Search widgets"
            />
          </div>

          <div className="add-widget-modal__list">
          {filteredCatalog.length === 0 && (
            <div className="add-widget-modal__empty">
              {search ? 'No widgets match your search' : 'No widgets available'}
            </div>
          )}

          {filteredCatalog.map((widget) => {
            const icon = getWidgetIcon(widget);
            return (
              <div key={widget.id} className="add-widget-modal__item">
                <div className="add-widget-modal__item-icon">
                  {icon.type === 'image' ? (
                    <img
                      src={icon.value}
                      alt=""
                      className="add-widget-modal__item-icon-img"
                      loading="lazy"
                    />
                  ) : (
                    <FontAwesomeIcon icon={icon.value} />
                  )}
                </div>
              <div className="add-widget-modal__item-info">
                <span className="add-widget-modal__item-name">
                  {widget.display_name || widget.id}
                </span>
                <span className="add-widget-modal__item-desc">
                  {widget.description || 'No description'}
                </span>
                {widget.verified && (
                  <span className="add-widget-modal__verified">
                    <FontAwesomeIcon icon={faCheck} /> Verified
                  </span>
                )}
              </div>
                <button
                  className="add-widget-modal__item-add"
                  onClick={() => handleAdd(widget.id)}
                  title={`Add ${widget.display_name || widget.id}`}
                >
                  <FontAwesomeIcon icon={faPlus} />
                  <span>Add</span>
                </button>
              </div>
            );
          })}
          </div>
        </div>
      </div>
    </div>,
    getModalRoot()
  );
}
