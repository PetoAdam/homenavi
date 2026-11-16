// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import WidgetSettingsModal from './WidgetSettingsModal';
import IntegrationIFrameWidget from './widgets/IntegrationIFrameWidget';
import GroupControlsWidget from './widgets/GroupControlsWidget';

const request = vi.hoisted(() => vi.fn());
const fixtures = vi.hoisted(() => ({ groups: [] }));
vi.mock('../../../services/httpClient', () => ({ default: { get: request } }));
vi.mock('../../../context/AuthContext', () => ({ useAuth: () => ({ user: { role: 'resident' }, accessToken: 'token' }) }));
vi.mock('../../../hooks/useDeviceHubDevices', () => ({ default: () => ({ devices: [], connectionInfo: { commandsReady: true } }) }));
vi.mock('../../../hooks/useErsInventory', () => ({ default: () => ({ devices: [], groups: fixtures.groups, loading: false }) }));
vi.mock('../../common/BaseModal/BaseModal', () => ({ default: ({ children }) => <div>{children}</div> }));
vi.mock('../../common/GlassSelect/GlassSelect', () => ({ default: ({ value, options, onChange, ariaLabel }) => (
  <select value={value} aria-label={ariaLabel} onChange={(event) => onChange(event.target.value)}>
    <option value="">Select</option>
    {options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
  </select>
) }));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('integration widget device settings', () => {
  let root;
  let container;

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
    request.mockReset();
    fixtures.groups = [];
  });

  it.each([
    ['integration.connector.device', 'connector', [{ id: 'blind/2', name: 'Bedroom blind' }], 'Bedroom blind'],
    ['integration.lg_thinq.device', 'lg-thinq', { 'lg/2': { name: 'Living room TV' } }, 'Living room TV'],
  ])('loads and saves a selected device for %s', async (widgetType, integration, devices, name) => {
    request.mockResolvedValue({ success: true, data: { devices } });
    const save = vi.fn();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => root.render(<WidgetSettingsModal open widgetItem={{ instance_id: 'widget', widget_type: widgetType, settings: {} }} onSave={save} onClose={() => {}} />));
    expect(request).toHaveBeenCalledWith(`/integrations/${integration}/api/realtime/snapshot`, { token: 'token' });
    const select = container.querySelector('select[aria-label="Select a device"]');
    expect(select.textContent).toContain(name);
    const id = integration === 'connector' ? 'blind/2' : 'lg/2';
    await act(async () => {
      select.value = id;
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    act(() => Array.from(container.querySelectorAll('button')).find((button) => button.textContent.includes('Save')).click());
    expect(save).toHaveBeenCalledWith('widget', expect.objectContaining({ device_id: id }));
  });

  it('passes the device ID to the iframe without discarding existing query parameters', () => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<IntegrationIFrameWidget widgetType="integration.connector.device" meta={{ entry_url: '/integrations/connector/widgets/connector-device/?test=1' }} settings={{ device_id: 'blind/2 & 3' }} />));
    const src = new URL(container.querySelector('iframe').src);
    expect(src.searchParams.get('test')).toBe('1');
    expect(src.searchParams.get('device_id')).toBe('blind/2 & 3');
  });

  it('saves the group field view and applies it to controls and metrics', () => {
    fixtures.groups = [{ id: 'group', name: 'Room', devices: [{ id: 'device', state: { temperature: 20, target: 22 }, inputs: [{ id: 'target', property: 'target', type: 'number' }] }] }];
    const save = vi.fn();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<WidgetSettingsModal open widgetItem={{ instance_id: 'widget', widget_type: 'homenavi.group.controls', settings: { group_id: 'group', fields: ['temperature'] } }} onSave={save} onClose={() => {}} />));
    act(() => Array.from(container.querySelectorAll('[aria-label="Field view"] button')).find(button => button.textContent.includes('List')).click());
    act(() => Array.from(container.querySelectorAll('button')).find(button => button.textContent.includes('Save')).click());
    expect(save).toHaveBeenCalledWith('widget', expect.objectContaining({ fields_layout: 'list' }));
    act(() => root.render(<GroupControlsWidget settings={{ group_id: 'group', fields: ['temperature'], fields_layout: 'list' }} />));
    expect(container.querySelector('.dcr-layout-list')).not.toBeNull();
    expect(container.querySelector('.dmr-layout-list')).not.toBeNull();
    act(() => root.render(<GroupControlsWidget settings={{ group_id: 'group', fields: ['temperature'] }} />));
    expect(container.querySelector('.dcr-layout-cards')).not.toBeNull();
    expect(container.querySelector('.dmr-layout-cards')).not.toBeNull();
  });
});