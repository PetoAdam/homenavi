// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';

const render = vi.hoisted(() => vi.fn());
const loadTracing = vi.hoisted(() => vi.fn());

vi.mock('react-dom/client', () => ({ createRoot: () => ({ render }) }));
vi.mock('./App.jsx', () => ({ default: () => null }));
vi.mock('./services/tracing', () => {
  loadTracing();
  return new Promise(() => {});
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.resetModules();
  render.mockClear();
  loadTracing.mockClear();
});

it('does not load browser tracing when the collector is disabled', async () => {
  vi.stubEnv('VITE_OTEL_BROWSER_ENABLED', 'true');
  const json = vi.fn().mockResolvedValue({ enabled: false });
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json }));
  document.body.innerHTML = '<div id="root"></div>';
  await import('./main.jsx');
  await vi.waitFor(() => expect(json).toHaveBeenCalledOnce());
  expect(render).toHaveBeenCalledOnce();
  expect(loadTracing).not.toHaveBeenCalled();
});

it('renders without waiting for browser tracing to load', async () => {
  vi.stubEnv('VITE_OTEL_BROWSER_ENABLED', 'true');
  vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})));
  document.body.innerHTML = '<div id="root"></div>';
  await import('./main.jsx');
  expect(render).toHaveBeenCalledOnce();
});

it('loads tracing when the gateway enables it', async () => {
  vi.stubEnv('VITE_OTEL_BROWSER_ENABLED', 'true');
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ enabled: true }) }));
  document.body.innerHTML = '<div id="root"></div>';
  await import('./main.jsx');
  await vi.waitFor(() => expect(loadTracing).toHaveBeenCalledOnce());
  expect(render).toHaveBeenCalledOnce();
});