import React from 'react';
import { getWidgetComponent, getWidgetTypeMeta } from './widgetRegistry';
import UnknownWidget from './widgets/UnknownWidget';
import IntegrationIFrameWidget from './widgets/IntegrationIFrameWidget';

/**
 * WidgetRenderer - Maps widget types to their respective components
 * 
 * This component acts as a registry for all available widget types.
 * Third-party widgets can be added here via the integration system.
 */

export default function WidgetRenderer({
  instanceId,
  widgetType,
  settings,
  catalog,
  enabled,
  editMode,
  onSettings,
  onSaveSettings,
  onRemove,
}) {
  const meta = getWidgetTypeMeta(widgetType, catalog);

  if (meta?.source === 'integration') {
    return (
      <IntegrationIFrameWidget
        instanceId={instanceId}
        widgetType={widgetType}
        meta={meta}
        settings={settings}
        enabled={enabled}
        editMode={editMode}
        onSettings={onSettings}
        onSaveSettings={onSaveSettings}
        onRemove={onRemove}
      />
    );
  }

  // Find the widget component
  const WidgetComponent = getWidgetComponent(widgetType);

  if (!WidgetComponent) {
    return (
      <UnknownWidget
        instanceId={instanceId}
        widgetType={widgetType}
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
      />
    );
  }

  return (
    <WidgetComponent
      instanceId={instanceId}
      settings={settings}
      enabled={enabled}
      editMode={editMode}
      onSettings={onSettings}
      onSaveSettings={onSaveSettings}
      onRemove={onRemove}
    />
  );
}
