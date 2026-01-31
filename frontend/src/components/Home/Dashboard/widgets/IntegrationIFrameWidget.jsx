import React, { useCallback, useMemo, useState } from 'react';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';

export default function IntegrationIFrameWidget({
  instanceId,
  widgetType,
  meta,
  settings,
  editMode,
  onSettings,
  onRemove,
}) {
  const [frameError, setFrameError] = useState('');

  const src = useMemo(() => {
    const u = meta?.entry_url || meta?.entry?.url;
    if (typeof u === 'string' && u.startsWith('/')) return u;
    return null;
  }, [meta]);

  const onFrameLoad = useCallback((e) => {
    try {
      const doc = e?.currentTarget?.contentDocument;
      if (!doc) return;

      const title = (doc.title || '').toLowerCase().trim();
      const hasSpaRoot = !!doc.getElementById('root');
      const hasModalRoot = !!doc.getElementById('modal-root');
      // If the integration iframe receives the Homenavi SPA (usually from SW fallback),
      // render a friendly error instead of showing a recursive dashboard.
      if (title === 'homenavi' || (hasSpaRoot && hasModalRoot)) {
        setFrameError('This integration widget failed to load (received the Homenavi app shell). Try hard-refreshing or unregistering the service worker.');
      } else {
        setFrameError('');
      }
    } catch {
      // ignore cross-origin/sandbox access issues
    }
  }, []);

  return (
    <WidgetShell
      title={undefined}
      subtitle={undefined}
      showHeader={false}
      flush
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      interactive={!editMode}
    >
      {!src ? (
        <div style={{ opacity: 0.85, padding: '0.75rem' }}>
          Unable to render this integration widget (missing iframe URL).
        </div>
      ) : frameError ? (
        <div style={{ padding: '0.9rem', opacity: 0.9 }}>
          <div style={{ fontWeight: 700, marginBottom: '0.35rem' }}>{meta?.display_name || 'Integration widget'}</div>
          <div style={{ fontSize: '0.95rem', opacity: 0.9 }}>{frameError}</div>
        </div>
      ) : (
        <iframe
          title={meta?.display_name || widgetType}
          src={src}
          onLoad={onFrameLoad}
          style={{ width: '100%', height: '100%', border: 0, background: 'transparent' }}
            sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation"
          referrerPolicy="no-referrer"
        />
      )}
    </WidgetShell>
  );
}

IntegrationIFrameWidget.defaultHeight = 5;
