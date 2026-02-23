import { describe, expect, it } from 'vitest';
import { hasSetupUiPath, setupRouteForIntegration } from './integrationSetup';

describe('integration setup ui helpers', () => {
  it('returns true only when setup_ui_path is present', () => {
    expect(hasSetupUiPath({ id: 'lg-thinq', setup_ui_path: '/ui/setup.html' })).toBe(true);
    expect(hasSetupUiPath({ id: 'lg-thinq', setup_ui_path: '  /ui/setup.html  ' })).toBe(true);
    expect(hasSetupUiPath({ id: 'lg-thinq', setup_ui_path: '' })).toBe(false);
    expect(hasSetupUiPath({ id: 'lg-thinq', default_ui_path: '/ui/dashboard.html' })).toBe(false);
    expect(hasSetupUiPath({ id: 'lg-thinq' })).toBe(false);
  });

  it('builds setup route only when setup_ui_path exists', () => {
    expect(setupRouteForIntegration({ id: 'lg-thinq', setup_ui_path: '/ui/setup.html' })).toBe('/apps/lg-thinq/setup');
    expect(setupRouteForIntegration({ id: 'lg-thinq', default_ui_path: '/ui/dashboard.html' })).toBe('');
    expect(setupRouteForIntegration({ setup_ui_path: '/ui/setup.html' })).toBe('');
  });
});
