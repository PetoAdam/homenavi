// @vitest-environment jsdom

import { afterEach, describe, expect, it } from 'vitest';

import { isPublicDemoModeEnabled } from './demoMode';

const originalDemoMode = import.meta.env.VITE_DEMO_MODE;

afterEach(() => {
  if (typeof window !== 'undefined') {
    delete window.__HOMENAVI_RUNTIME_CONFIG__;
  }
  if (typeof originalDemoMode === 'undefined') {
    delete import.meta.env.VITE_DEMO_MODE;
  } else {
    import.meta.env.VITE_DEMO_MODE = originalDemoMode;
  }
});

describe('isPublicDemoModeEnabled', () => {
  it('returns false for undefined, empty, and explicit false-like values', () => {
    delete import.meta.env.VITE_DEMO_MODE;
    expect(isPublicDemoModeEnabled()).toBe(false);

    for (const value of ['', '   ', 'false', 'FALSE', '0', 'no', 'off']) {
      import.meta.env.VITE_DEMO_MODE = value;
      expect(isPublicDemoModeEnabled()).toBe(false);
    }
  });

  it('returns true for explicit true-like values', () => {
    for (const value of ['true', 'TRUE', '1', 'yes', 'on']) {
      import.meta.env.VITE_DEMO_MODE = value;
      expect(isPublicDemoModeEnabled()).toBe(true);
    }
  });

	it('prefers runtime config over build-time config', () => {
	  import.meta.env.VITE_DEMO_MODE = 'false';
	  window.__HOMENAVI_RUNTIME_CONFIG__ = { VITE_DEMO_MODE: true };
	  expect(isPublicDemoModeEnabled()).toBe(true);
	});
});