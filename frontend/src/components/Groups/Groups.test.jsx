// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Groups from './Groups';

const fixtures = vi.hoisted(() => ({
  navigate: vi.fn(),
  openEdit: vi.fn(),
  sendCommand: vi.fn().mockResolvedValue({ success: true }),
  groupId: '',
  devices: [],
  groups: [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices: [] }],
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => fixtures.navigate,
  useParams: () => ({ groupId: fixtures.groupId }),
}));
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => ({ user: { role: 'resident' }, accessToken: 'token', bootstrapping: false }),
}));
vi.mock('../../hooks/useDeviceHubDevices', () => ({ default: () => ({ devices: fixtures.devices }) }));
vi.mock('../../services/deviceHubService', () => ({ sendDeviceCommand: fixtures.sendCommand }));
vi.mock('../../hooks/useErsInventory', () => ({
  default: () => ({ devices: fixtures.devices, groups: fixtures.groups, loading: false, refresh: vi.fn() }),
}));
vi.mock('./useGroupsEditorActions', () => ({
  useGroupsEditorActions: () => ({ openCreate: vi.fn(), openEdit: fixtures.openEdit, openDelete: vi.fn() }),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('Groups', () => {
  let root;
  let container;

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
    fixtures.groupId = '';
    fixtures.groups = [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices: [] }];
    vi.clearAllMocks();
  });

  it('uses the interactive device-card styling without changing card actions', () => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<Groups />));

    const tile = container.querySelector('.group-tile-card');
    expect(tile.classList.contains('device-tile-card')).toBe(true);
    expect(tile.classList.contains('no-hover')).toBe(false);
    act(() => tile.querySelector('.device-title').click());
    expect(fixtures.navigate).toHaveBeenCalledWith('/groups/living-room');
    fixtures.navigate.mockClear();
    act(() => tile.querySelector('[aria-label="Edit group"]').click());
    expect(fixtures.openEdit).toHaveBeenCalledOnce();
    expect(fixtures.navigate).not.toHaveBeenCalled();
  });

  it('uses saved presentation for shared inferred controls', () => {
    const member = id => ({
      id, state: { target: 20 },
      capabilities: [{ id: 'target', property: 'target', kind: 'numeric', value_type: 'number', range: { min: 5, max: 35, step: 0.5 }, access: { write: true } }],
      meta: { presentation: { controls: { target: 'stepper' } } },
    });
    fixtures.groupId = 'living-room';
    fixtures.groups = [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices: [member('demo/1'), member('demo/2')] }];
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<Groups />));
    expect(container.querySelector('.groups-shared-controls-panel .dcr-stepper')).not.toBeNull();
  });

  it('does not expose group controls with different command properties', () => {
    const devices = ['heat', 'cool'].map((property, index) => ({
      id: `demo/${index}`, state: { [property]: 20 },
      inputs: [{ id: 'set_target', property, label: 'Target', type: 'number', range: { min: 0, max: 30, step: 1 } }],
    }));
    fixtures.groupId = 'living-room';
    fixtures.groups = [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices }];
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<Groups />));
    expect(container.querySelector('.groups-shared-controls-panel input[type="number"]')).toBeNull();
    expect(fixtures.sendCommand).not.toHaveBeenCalled();
  });

  it('sends a common step to every group member', async () => {
    const member = (id, step) => ({
      id, state: { target: 0 },
      inputs: [{ id: 'target', property: 'target', label: 'Target', type: 'number', range: { min: 0, max: 30, step }, ui: { control: 'stepper' } }],
    });
    fixtures.groupId = 'living-room';
    fixtures.groups = [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices: [member('demo/1', 2), member('demo/2', 3)] }];
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<Groups />));
    const panel = container.querySelector('.groups-shared-controls-panel');
    expect(panel.querySelector('input[type="number"]').step).toBe('6');
    await act(async () => panel.querySelector('[aria-label="Increase Target"]').click());
    expect(fixtures.sendCommand).toHaveBeenCalledTimes(2);
    expect(fixtures.sendCommand).toHaveBeenCalledWith('demo/1', { state: { target: 6 } }, 'token');
    expect(fixtures.sendCommand).toHaveBeenCalledWith('demo/2', { state: { target: 6 } }, 'token');
  });
});