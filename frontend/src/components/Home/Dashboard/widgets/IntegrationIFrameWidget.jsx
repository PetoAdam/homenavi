import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  const frameRef = useRef(null);
  const iframeObserverRef = useRef(null);
  const iframeWindowResizeHandlerRef = useRef(null);

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

      if (!instanceId) return;

      const emitDesiredHeight = () => {
        const iframe = frameRef.current;
        const innerDoc = iframe?.contentDocument;
        if (!innerDoc) return;
        const body = innerDoc.body;
        const root = innerDoc.documentElement;
        if (!body || !root) return;

        const contentHeight = Math.max(
          body.scrollHeight || 0,
          body.offsetHeight || 0,
          root.scrollHeight || 0,
          root.offsetHeight || 0,
        );
        if (!Number.isFinite(contentHeight) || contentHeight <= 0) return;

        const padded = Math.max(220, Math.ceil(contentHeight + 8));
        window.dispatchEvent(new CustomEvent('homenavi:widgetDesiredHeight', {
          detail: { instanceId, heightPx: padded },
        }));
      };

      emitDesiredHeight();

      if (iframeObserverRef.current) {
        iframeObserverRef.current.disconnect();
        iframeObserverRef.current = null;
      }
      if (iframeWindowResizeHandlerRef.current) {
        window.removeEventListener('resize', iframeWindowResizeHandlerRef.current);
        iframeWindowResizeHandlerRef.current = null;
      }

      if (window.ResizeObserver) {
        iframeObserverRef.current = new ResizeObserver(() => emitDesiredHeight());
        iframeObserverRef.current.observe(doc.documentElement);
        if (doc.body) iframeObserverRef.current.observe(doc.body);
      }

      iframeWindowResizeHandlerRef.current = () => emitDesiredHeight();
      window.addEventListener('resize', iframeWindowResizeHandlerRef.current);
    } catch {
      // ignore cross-origin/sandbox access issues
    }
  }, [instanceId]);

  useEffect(() => {
    return () => {
      if (iframeObserverRef.current) {
        iframeObserverRef.current.disconnect();
        iframeObserverRef.current = null;
      }
      if (iframeWindowResizeHandlerRef.current) {
        window.removeEventListener('resize', iframeWindowResizeHandlerRef.current);
        iframeWindowResizeHandlerRef.current = null;
      }
    };
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
          ref={frameRef}
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
