import { describe, expect, it } from 'vitest';
import { ignoredBrowserTraceUrls, initializeBrowserTracing } from './tracing';

describe('browser tracing configuration', () => {
  it('only instruments same-origin API requests and excludes the collector', () => {
    const [ignored] = ignoredBrowserTraceUrls('https://home.example.test');
    expect(ignored.test('https://home.example.test/api/hdp/devices?secret=value')).toBe(false);
    expect(ignored.test('https://home.example.test/api/telemetry/traces')).toBe(true);
    expect(ignored.test('https://home.example.test/api/telemetry/traces?token=x')).toBe(true);
    expect(ignored.test('https://homeXexampleXtest/api/devices')).toBe(true);
    expect(ignored.test('https://external.example/api/devices')).toBe(true);
    expect(ignored.test('https://home.example.test/assets/logo.png')).toBe(true);
  });

  it('is disabled unless explicitly enabled', () => {
    expect(initializeBrowserTracing({ enabled: false })).toBeNull();
  });
});