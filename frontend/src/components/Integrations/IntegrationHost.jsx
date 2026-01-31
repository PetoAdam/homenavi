import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import { getIntegrationRegistry } from '../../services/integrationService';
import PageHeader from '../common/PageHeader/PageHeader';
import GlassCard from '../common/GlassCard/GlassCard';
import { useAuth } from '../../context/AuthContext';
import './IntegrationHost.css';

function resolveIframeSrc({ integrationId, defaultUIPath, routeSubPath }) {
  const sub = (routeSubPath || '').trim();
  if (sub && sub !== '/') {
    return `/integrations/${integrationId}${sub.startsWith('/') ? '' : '/'}${sub}`;
  }

  const p = (defaultUIPath || '/ui/').trim();
  if (p.startsWith('/integrations/')) return p;
  if (p.startsWith('/')) return `/integrations/${integrationId}${p}`;
  return `/integrations/${integrationId}/${p}`;
}

export default function IntegrationHost() {
  const { integrationId } = useParams();
  const location = useLocation();
  const { accessToken, user } = useAuth();
  const [registry, setRegistry] = useState(null);
  const [error, setError] = useState('');
  const [frameError, setFrameError] = useState('');

  useEffect(() => {
    let alive = true;
    if (!accessToken || !user) {
      setRegistry(null);
      setError('');
      return () => { alive = false; };
    }
    (async () => {
      const res = await getIntegrationRegistry();
      if (!alive) return;
      if (!res.success) {
        setError(res.error || 'Failed to load integrations');
        setRegistry(null);
        return;
      }
      setError('');
      setRegistry(res.data);
    })();
    return () => { alive = false; };
  }, [accessToken, user, integrationId]);

  const integration = useMemo(() => {
    const list = registry?.integrations;
    if (!Array.isArray(list)) return null;
    return list.find((i) => i?.id === integrationId) || null;
  }, [registry, integrationId]);

  const routeSubPath = useMemo(() => {
    const prefix = `/apps/${integrationId}`;
    if (!location.pathname.startsWith(prefix)) return '';
    return location.pathname.slice(prefix.length);
  }, [location.pathname, integrationId]);

  const iframeSrc = useMemo(() => {
    return resolveIframeSrc({
      integrationId,
      defaultUIPath: integration?.default_ui_path,
      routeSubPath,
    });
  }, [integrationId, integration?.default_ui_path, routeSubPath]);

  const onFrameLoad = useCallback((e) => {
    try {
      const doc = e?.currentTarget?.contentDocument;
      if (!doc) return;
      const title = (doc.title || '').toLowerCase().trim();
      const hasSpaRoot = !!doc.getElementById('root');
      const hasModalRoot = !!doc.getElementById('modal-root');
      if (title === 'homenavi' || (hasSpaRoot && hasModalRoot)) {
        setFrameError('This integration page failed to load (received the Homenavi app shell). Try hard-refreshing or unregistering the service worker.');
      } else {
        setFrameError('');
      }
    } catch {
      // ignore cross-origin/sandbox access issues
    }
  }, []);

  if (!integrationId) return null;

  if (error) {
    return (
      <GlassCard className="integration-host" interactive={false}>
        <div style={{ fontWeight: 700, marginBottom: '0.5rem' }}>Integration error</div>
        <div style={{ opacity: 0.9 }}>{error}</div>
      </GlassCard>
    );
  }

  if (!integration) {
    return (
      <GlassCard className="integration-host" interactive={false}>
        <div style={{ fontWeight: 700, marginBottom: '0.5rem' }}>Integration not found</div>
        <div style={{ opacity: 0.9 }}>No installed integration with id "{integrationId}".</div>
      </GlassCard>
    );
  }

  return (
    <div className="integration-page">
      <PageHeader
        title={integration.display_name || integration.id}
      />

      {frameError ? (
        <GlassCard interactive={false}>
          <div style={{ fontWeight: 700, marginBottom: '0.35rem' }}>Unable to load integration</div>
          <div style={{ opacity: 0.9 }}>{frameError}</div>
        </GlassCard>
      ) : (
        <iframe
          title={integration.display_name || integration.id}
          src={iframeSrc}
          onLoad={onFrameLoad}
          style={{ width: '100%', height: 'calc(100vh - 220px)', minHeight: '78vh', border: '0', borderRadius: '16px', background: 'transparent' }}
            sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation"
          referrerPolicy="no-referrer"
        />
      )}
    </div>
  );
}
