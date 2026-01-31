import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGripVertical, faCog, faTrash } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../GlassCard/GlassCard';
import NoPermissionWidget from '../NoPermissionWidget/NoPermissionWidget';
import './WidgetShell.css';

/**
 * WidgetShell - Base wrapper for all dashboard widgets
 * 
 * Provides:
 * - Consistent styling
 * - Loading state
 * - Error handling (401/403)
 * - Edit mode overlay with drag handle, settings, and remove buttons
 */
export default function WidgetShell({
  title,
  icon,
  subtitle,
  children,
  loading = false,
  error = null,
  status = null, // 401, 403, or null
  editMode = false,
  onSettings,
  onRemove,
  className = '',
  interactive = true,
  flush = false,
  showHeader = true,
  ...props
}) {
  // Handle 401/403 errors
  if (status === 401) {
    return (
      <GlassCard className={`widget-shell widget-shell--error ${className}`} interactive={false} {...props}>
        <NoPermissionWidget
          title={title || 'Access Required'}
          message="Please sign in to view this widget."
          showLogin
        />
      </GlassCard>
    );
  }

  if (status === 403) {
    return (
      <GlassCard className={`widget-shell widget-shell--error ${className}`} interactive={false} {...props}>
        <NoPermissionWidget
          title={title || 'No Permission'}
          message="You don't have permission to view this widget."
          showLogin={false}
        />
      </GlassCard>
    );
  }

  return (
    <GlassCard
      className={`widget-shell ${flush ? 'glass-card--flush' : ''} ${editMode ? 'widget-shell--edit-mode' : ''} ${className}`}
      interactive={interactive && !editMode}
      {...props}
    >
      {/* Edit mode overlay */}
      {editMode && (
        <div className="widget-shell__edit-overlay">
          <div className="widget-shell__drag-handle" title="Drag to move">
            <FontAwesomeIcon icon={faGripVertical} />
          </div>
          <div className="widget-shell__edit-actions">
            {onSettings && (
              <button
                className="widget-shell__edit-btn widget-shell__edit-btn--settings"
                onClick={(e) => {
                  e.stopPropagation();
                  onSettings();
                }}
                title="Widget settings"
              >
                <FontAwesomeIcon icon={faCog} />
              </button>
            )}
            {onRemove && (
              <button
                className="widget-shell__edit-btn widget-shell__edit-btn--remove"
                onClick={(e) => {
                  e.stopPropagation();
                  onRemove();
                }}
                title="Remove widget"
              >
                <FontAwesomeIcon icon={faTrash} />
              </button>
            )}
          </div>
        </div>
      )}

      {/* Widget content wrapper */}
      <div className={`widget-shell__content ${editMode ? 'widget-shell__content--muted' : ''}`}>
        {/* Optional header */}
        {showHeader && (title || subtitle) && (
          <div className="widget-shell__header">
            {title && (
              <div className="widget-shell__title-row">
                {icon && <FontAwesomeIcon icon={icon} className="widget-shell__title-icon" />}
                <div className="widget-shell__title">{title}</div>
              </div>
            )}
            {subtitle && <div className="widget-shell__subtitle">{subtitle}</div>}
          </div>
        )}

        {/* Loading state */}
        {loading && (
          <div className="widget-shell__loading">
            <div className="widget-shell__spinner" />
          </div>
        )}

        {/* Error state */}
        {error && !loading && (
          <div className="widget-shell__error-message">
            {error}
          </div>
        )}

        {/* Main content */}
        {!loading && !error && children}
      </div>
    </GlassCard>
  );
}
