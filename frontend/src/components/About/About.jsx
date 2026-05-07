import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowsRotate,
  faArrowUpRightFromSquare,
  faBoxOpen,
  faCircleInfo,
  faClock,
  faCube,
  faGlobe,
  faHeartPulse,
  faLayerGroup,
  faLanguage,
  faLaptop,
  faPlug,
  faRocket,
  faServer,
  faShieldHalved,
  faTriangleExclamation,
  faUser,
  faWifi,
} from '@fortawesome/free-solid-svg-icons';
import PageHeader from '../common/PageHeader/PageHeader';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassPill from '../common/GlassPill/GlassPill';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import { useAuth } from '../../context/AuthContext';
import { getIntegrationRegistry } from '../../services/integrationService';
import http from '../../services/httpClient';
import './About.css';

const BUILD = typeof __HOMENAVI_BUILD__ === 'object' && __HOMENAVI_BUILD__
  ? __HOMENAVI_BUILD__
  : { version: 'dev', releaseTag: 'dev', commit: '', builtAt: '' };

function getInstallMode() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'Browser';
  if (window.matchMedia('(display-mode: standalone)').matches) return 'Installed app';
  if (window.matchMedia('(display-mode: minimal-ui)').matches) return 'Minimal UI';
  return 'Browser';
}

function formatBuildTime(value) {
  if (!value) return 'Unknown';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function externalLink(title, href, description) {
  return { title, href, description };
}

function formatCheckedAt(value) {
  if (!value) return 'Not checked yet';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Not checked yet';
  return date.toLocaleTimeString();
}

function latencyLabel(value) {
  return Number.isFinite(value) ? `${value} ms` : 'n/a';
}

function sleep(ms) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

function statusTone(status) {
  if (status === 'ok') return 'success';
  if (status === 'degraded') return 'warning';
  return 'danger';
}

function summarizeGatewayStatus(service) {
  if (!service) {
    return { tone: 'default', text: 'Checking gateway', meta: 'Gateway probe pending' };
  }
  if (service.status === 'ok') {
    return { tone: 'success', text: 'Gateway reachable', meta: service.detail || 'Healthy response received' };
  }
  if (service.status === 'degraded') {
    return { tone: 'warning', text: 'Gateway restricted', meta: service.detail || service.summary || 'Access limited' };
  }
  return { tone: 'danger', text: 'Gateway unavailable', meta: service.detail || service.summary || 'Request failed' };
}

function summarizeDownstreamStatus(services, loading) {
  if (loading && services.length === 0) {
    return { tone: 'default', text: 'Checking downstream', meta: 'Service probes pending' };
  }
  if (services.length === 0) {
    return { tone: 'default', text: 'No downstream checks', meta: 'No routed services probed' };
  }
  if (services.some((service) => service.status === 'down')) {
    return { tone: 'danger', text: 'Downstream failures', meta: 'One or more routed services are down' };
  }
  if (services.some((service) => service.status === 'degraded')) {
    return { tone: 'warning', text: 'Downstream restricted', meta: 'Some routed services need elevated access' };
  }
  return { tone: 'success', text: 'Downstream healthy', meta: 'All routed services responded' };
}

async function probeHealthEndpoint(name, url, token) {
  let lastResponse = null;
  let latencyMs = null;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const started = globalThis.performance?.now?.() ?? Date.now();
    const response = await http.get(url, { token, timeout: 4000 });
    const finished = globalThis.performance?.now?.() ?? Date.now();
    latencyMs = Math.max(0, Math.round(finished - started));

    if (response.success) {
      return {
        name,
        status: 'ok',
        tone: 'success',
        summary: 'Reachable',
        detail: attempt === 0 ? 'Healthy response received' : 'Healthy after retry',
        latencyMs,
      };
    }

    lastResponse = response;
    if (![502, 503, 504].includes(response.status) || attempt === 1) {
      break;
    }
    await sleep(700);
  }

  return {
    name,
    status: 'down',
    tone: 'danger',
    summary: lastResponse?.status ? `HTTP ${lastResponse.status}` : 'Unavailable',
    detail: lastResponse?.error || 'Request failed',
    latencyMs,
  };
}

export default function About() {
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const [integrationCount, setIntegrationCount] = useState(null);
  const [integrationNames, setIntegrationNames] = useState([]);
  const [backendHealth, setBackendHealth] = useState({ loading: true, checkedAt: '', services: [] });

  const coreHealthChecks = useMemo(() => ([
    { name: 'API Gateway', url: '/api/gateway/health', access: 'public' },
    { name: 'Auth Service', url: '/api/auth/health', access: 'auth' },
    { name: 'User Service', url: '/api/users/health', access: 'resident' },
    { name: 'Dashboard Service', url: '/api/dashboard/health', access: 'resident' },
    { name: 'Device Hub', url: '/api/hdp/health', access: 'resident' },
    { name: 'Entity Registry', url: '/api/ers/health', access: 'resident' },
    { name: 'History Service', url: '/api/history/health', access: 'resident' },
    { name: 'Automation Service', url: '/api/automation/health', access: 'resident' },
    { name: 'Weather Service', url: '/api/weather/health', access: 'resident' },
    { name: 'Integration Proxy', url: '/integrations/healthz', access: 'public' },
  ]), []);

  useEffect(() => {
    let cancelled = false;

    async function loadIntegrations() {
      if (!isResidentOrAdmin || !accessToken) {
        setIntegrationCount(null);
        setIntegrationNames([]);
        return;
      }
      const response = await getIntegrationRegistry();
      if (!response.success || cancelled) return;
      const items = Array.isArray(response.data?.integrations) ? response.data.integrations : [];
      setIntegrationCount(items.length);
      setIntegrationNames(
        items
          .map(item => item?.display_name || item?.id || '')
          .filter(Boolean)
          .slice(0, 6),
      );
    }

    loadIntegrations();
    return () => {
      cancelled = true;
    };
  }, [accessToken, isResidentOrAdmin]);

  const refreshBackendHealth = useCallback(async () => {
    setBackendHealth((prev) => ({ ...prev, loading: true }));

    const checks = coreHealthChecks.map((check) => {
      if (check.access === 'public') {
        return probeHealthEndpoint(check.name, check.url);
      }
      if (check.access === 'auth') {
        if (!accessToken) {
          return Promise.resolve({
            name: check.name,
            status: 'degraded',
            tone: 'warning',
            summary: 'Restricted',
            detail: 'Authentication required',
            latencyMs: null,
          });
        }
        return probeHealthEndpoint(check.name, check.url, accessToken);
      }
      if (!isResidentOrAdmin || !accessToken) {
        return Promise.resolve({
          name: check.name,
          status: 'degraded',
          tone: 'warning',
          summary: 'Restricted',
          detail: 'Resident or admin access required',
          latencyMs: null,
        });
      }
      return probeHealthEndpoint(check.name, check.url, accessToken);
    });

    const services = await Promise.all(checks);
    setBackendHealth({
      loading: false,
      checkedAt: new Date().toISOString(),
      services,
    });
  }, [accessToken, coreHealthChecks, isResidentOrAdmin]);

  useEffect(() => {
    refreshBackendHealth();
  }, [refreshBackendHealth]);

  const runtimeFacts = useMemo(() => ([
    { label: 'Origin', value: typeof window !== 'undefined' ? window.location.origin : 'Unknown', icon: faGlobe },
    { label: 'Host', value: typeof window !== 'undefined' ? window.location.host : 'Unknown', icon: faLaptop },
    { label: 'Install mode', value: getInstallMode(), icon: faBoxOpen },
    { label: 'Language', value: typeof navigator !== 'undefined' ? navigator.language : 'Unknown', icon: faLanguage },
    { label: 'Timezone', value: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Unknown', icon: faGlobe },
    { label: 'Network', value: typeof navigator !== 'undefined' && navigator.onLine === false ? 'Offline' : 'Online', icon: faWifi },
  ]), []);

  const releaseFacts = useMemo(() => ([
    { label: 'Version', value: BUILD.version || 'dev', icon: faCube },
    { label: 'Release tag', value: BUILD.releaseTag || 'dev', icon: faLayerGroup },
    { label: 'Frontend build', value: formatBuildTime(BUILD.builtAt), icon: faCircleInfo },
  ]), []);

  const quickFacts = useMemo(() => ([
    { label: 'Signed in as', value: user?.user_name || user?.email || 'Unknown', icon: faUser },
    { label: 'Role', value: user?.role || 'Unknown', icon: faShieldHalved },
    { label: 'Integrations', value: integrationCount === null ? 'Unavailable' : String(integrationCount), icon: faPlug },
  ]), [integrationCount, user?.email, user?.role, user?.user_name]);

  const gatewayHealth = useMemo(
    () => backendHealth.services.find((service) => service.name === 'API Gateway') || null,
    [backendHealth.services],
  );

  const downstreamServices = useMemo(
    () => backendHealth.services.filter((service) => service.name !== 'API Gateway'),
    [backendHealth.services],
  );

  const gatewaySummary = useMemo(() => summarizeGatewayStatus(gatewayHealth), [gatewayHealth]);

  const downstreamSummary = useMemo(
    () => summarizeDownstreamStatus(downstreamServices, backendHealth.loading),
    [backendHealth.loading, downstreamServices],
  );

  const backendSummary = useMemo(() => {
    const services = Array.isArray(backendHealth.services) ? backendHealth.services : [];
    if (backendHealth.loading && services.length === 0) {
      return { tone: 'default', text: 'Checking backend health' };
    }
    if (gatewaySummary.tone === 'danger') {
      return { tone: 'danger', text: 'Gateway unavailable' };
    }
    if (downstreamSummary.tone === 'danger') {
      return { tone: 'warning', text: 'Gateway up, downstream failing' };
    }
    if (gatewaySummary.tone === 'warning' || downstreamSummary.tone === 'warning') {
      return { tone: 'warning', text: 'Gateway up, downstream restricted' };
    }
    return { tone: 'success', text: 'Backend reachable' };
  }, [backendHealth, downstreamSummary.tone, gatewaySummary.tone]);

  const heroStats = useMemo(() => ([
    { label: 'Release', value: BUILD.releaseTag || 'dev', meta: `Version ${BUILD.version || 'dev'}`, icon: faRocket },
    { label: 'Gateway', value: gatewaySummary.text, meta: gatewaySummary.meta, icon: faHeartPulse, tone: gatewaySummary.tone },
    { label: 'Downstream', value: downstreamSummary.text, meta: downstreamSummary.meta, icon: faServer, tone: downstreamSummary.tone },
    { label: 'Session', value: user?.role || 'Unknown', meta: user?.user_name || user?.email || 'Unknown user', icon: faUser },
  ]), [BUILD.releaseTag, BUILD.version, downstreamSummary.meta, downstreamSummary.text, downstreamSummary.tone, gatewaySummary.meta, gatewaySummary.text, gatewaySummary.tone, user?.email, user?.role, user?.user_name]);

  const links = useMemo(() => ([
    externalLink('GitHub repository', 'https://github.com/PetoAdam/homenavi', 'Source, issues, and release history.'),
    externalLink('Releases', 'https://github.com/PetoAdam/homenavi/releases', 'Published tags and release notes.'),
    externalLink('README', 'https://github.com/PetoAdam/homenavi/blob/main/README.md', 'Setup, architecture, and operations guide.'),
    externalLink('Issue tracker', 'https://github.com/PetoAdam/homenavi/issues', 'Bug reports and feature requests.'),
  ]), []);

  if (!bootstrapping && !user) {
    return <UnauthorizedView title="About" message="Sign in to view platform details." />;
  }

  if (!bootstrapping && !isResidentOrAdmin) {
    return <UnauthorizedView title="About" message="Resident or admin access is required to view deployment details." />;
  }

  return (
    <div className="about-page">
      <PageHeader
        title="About Homenavi"
        subtitle="Release identity, runtime context, and project references in one place."
      >
        <div className="about-page-pills">
          <GlassPill icon={faCube} text={`v${BUILD.version || 'dev'}`} tone="success" />
          <GlassPill icon={faPlug} text={integrationCount === null ? 'Integrations unavailable' : `${integrationCount} integrations`} tone="default" />
          <GlassPill icon={faWifi} text={typeof navigator !== 'undefined' && navigator.onLine === false ? 'Offline' : 'Online'} tone={typeof navigator !== 'undefined' && navigator.onLine === false ? 'warning' : 'success'} />
        </div>
      </PageHeader>

      <div className="about-layout">
        <GlassCard className="about-hero-card" interactive={false}>
          <div className="about-hero-shell">
            <div className="about-hero-copy">
              <span className="about-eyebrow">Deployment overview</span>
              <div className="about-brand-lockup">
                <div className="about-hero-mark about-hero-mark--brand">
                  <img className="about-brand-image" src="/icons/icon-192x192.png" alt="Homenavi" />
                </div>
                <div className="about-brand-copy">
                  <span>Homenavi</span>
                </div>
              </div>
              <div className="about-page-pills about-page-pills--hero">
                <GlassPill icon={faCube} text={`v${BUILD.version || 'dev'}`} tone="success" />
                <GlassPill icon={faPlug} text={integrationCount === null ? 'Integrations unavailable' : `${integrationCount} integrations`} tone="default" />
                <GlassPill icon={faWifi} text={typeof navigator !== 'undefined' && navigator.onLine === false ? 'Offline browser' : 'Browser online'} tone={typeof navigator !== 'undefined' && navigator.onLine === false ? 'warning' : 'success'} />
              </div>
            </div>
            <div className="about-hero-metrics" role="list" aria-label="Deployment summary metrics">
              {heroStats.map((fact) => (
                <div className={`about-hero-metric ${fact.tone ? `about-hero-metric--${fact.tone}` : ''}`} key={fact.label} role="listitem">
                  <span className="about-hero-metric-icon"><FontAwesomeIcon icon={fact.icon} /></span>
                  <div className="about-hero-metric-copy">
                    <span className="about-hero-metric-label">{fact.label}</span>
                    <strong className="about-hero-metric-value">{fact.value}</strong>
                    <span className="about-hero-metric-meta">{fact.meta}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </GlassCard>

        <div className="about-grid-shell">
          <GlassCard className="about-section-card about-section-card--release" interactive={false}>
            <div className="about-section-header">
              <h3>Release</h3>
              <span>Build identity</span>
            </div>
            <div className="about-fact-list">
              {releaseFacts.map(fact => (
                <div className="about-fact-row" key={fact.label}>
                  <span className="about-fact-icon"><FontAwesomeIcon icon={fact.icon} /></span>
                  <div className="about-fact-copy">
                    <span className="about-fact-label">{fact.label}</span>
                    <strong className="about-fact-value">{fact.value}</strong>
                  </div>
                </div>
              ))}
            </div>
          </GlassCard>

          <GlassCard className="about-section-card about-section-card--session" interactive={false}>
            <div className="about-section-header">
              <h3>Session</h3>
              <span>Current user context</span>
            </div>
            <div className="about-fact-list">
              {quickFacts.map(fact => (
                <div className="about-fact-row" key={fact.label}>
                  <span className="about-fact-icon"><FontAwesomeIcon icon={fact.icon} /></span>
                  <div className="about-fact-copy">
                    <span className="about-fact-label">{fact.label}</span>
                    <strong className="about-fact-value">{fact.value}</strong>
                  </div>
                </div>
              ))}
            </div>
            {integrationNames.length > 0 ? (
              <div className="about-chip-row">
                {integrationNames.map(name => (
                  <GlassPill key={name} text={name} tone="default" />
                ))}
              </div>
            ) : null}
          </GlassCard>

          <GlassCard className="about-section-card about-section-card--runtime" interactive={false}>
            <div className="about-section-header">
              <h3>Runtime</h3>
              <span>Browser and deployment context</span>
            </div>
            <div className="about-fact-list">
              {runtimeFacts.map(fact => (
                <div className="about-fact-row" key={fact.label}>
                  <span className="about-fact-icon"><FontAwesomeIcon icon={fact.icon} /></span>
                  <div className="about-fact-copy">
                    <span className="about-fact-label">{fact.label}</span>
                    <strong className="about-fact-value">{fact.value}</strong>
                  </div>
                </div>
              ))}
            </div>
          </GlassCard>

          <GlassCard className="about-section-card about-section-card--links" interactive={false}>
            <div className="about-section-header">
              <h3>Links</h3>
              <span>Project references and escape hatches</span>
            </div>
            <div className="about-link-list">
              {links.map(link => (
                <a className="about-link-item" href={link.href} target="_blank" rel="noreferrer" key={link.href}>
                  <div>
                    <strong>{link.title}</strong>
                    <span>{link.description}</span>
                  </div>
                  <FontAwesomeIcon icon={faArrowUpRightFromSquare} />
                </a>
              ))}
            </div>
          </GlassCard>

          <GlassCard className="about-section-card about-section-card--status" interactive={false}>
            <div className="about-section-header">
              <h3>Backend health</h3>
              <span>Live reachability for core platform services</span>
            </div>
            <div className="about-health-toolbar">
              <GlassPill icon={faServer} text={backendSummary.text} tone={backendSummary.tone} />
              <GlassPill icon={faHeartPulse} text={gatewaySummary.text} tone={gatewaySummary.tone} />
              <GlassPill icon={faServer} text={downstreamSummary.text} tone={downstreamSummary.tone} />
              <GlassPill icon={faClock} text={`Checked ${formatCheckedAt(backendHealth.checkedAt)}`} tone="default" />
              <GlassPill
                icon={backendHealth.loading ? faClock : faArrowsRotate}
                text={backendHealth.loading ? 'Refreshing…' : 'Refresh'}
                tone="default"
                onClick={() => {
                  refreshBackendHealth();
                }}
              />
            </div>
            <div className="about-status-grid">
              {backendHealth.services.map((service) => (
                <div className="about-status-item" key={service.name}>
                  <span className="about-fact-icon"><FontAwesomeIcon icon={service.status === 'down' ? faTriangleExclamation : faServer} /></span>
                  <div className="about-fact-copy">
                    <div className="about-status-head">
                      <span className="about-fact-label">{service.name}</span>
                      <GlassPill text={service.summary} tone={service.tone || statusTone(service.status)} className="about-status-pill" />
                    </div>
                    <strong className="about-fact-value">{service.detail}</strong>
                    <span className="about-status-meta">Latency {latencyLabel(service.latencyMs)}</span>
                  </div>
                </div>
              ))}
            </div>
          </GlassCard>
        </div>
      </div>
    </div>
  );
}