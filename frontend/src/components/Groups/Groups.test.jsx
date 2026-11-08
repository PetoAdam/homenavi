// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Groups from './Groups';

const fixtures = vi.hoisted(() => ({
  navigate: vi.fn(),
  openEdit: vi.fn(),
  devices: [],
  groups: [{ id: 'group-1', slug: 'living-room', name: 'Living room', devices: [] }],
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => fixtures.navigate,
  useParams: () => ({}),
}));
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => ({ user: { role: 'resident' }, accessToken: 'token', bootstrapping: false }),
}));
vi.mock('../../hooks/useDeviceHubDevices', () => ({ default: () => ({ devices: fixtures.devices }) }));
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
});