import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faQuestionCircle } from '@fortawesome/free-solid-svg-icons';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import './UnknownWidget.css';

/**
 * UnknownWidget - Fallback for unrecognized widget types
 * 
 * This is shown when a widget type from the backend doesn't have
 * a corresponding component in the registry. This can happen for:
 * - Third-party/integration widgets not yet installed
 * - Widget types from newer backend versions
 * - Corrupt/invalid widget type data
 */
function UnknownWidget({
  widgetType,
  editMode,
  onSettings,
  onRemove,
}) {
  return (
    <WidgetShell
      title="Unknown Widget"
      subtitle={widgetType}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      className="unknown-widget"
    >
      <div className="unknown-widget__content">
        <FontAwesomeIcon icon={faQuestionCircle} className="unknown-widget__icon" />
        <div className="unknown-widget__message">
          <span className="unknown-widget__title">Widget not available</span>
          <span className="unknown-widget__type">Type: {widgetType || 'unknown'}</span>
        </div>
      </div>
    </WidgetShell>
  );
}

UnknownWidget.defaultHeight = 4;

export default UnknownWidget;
