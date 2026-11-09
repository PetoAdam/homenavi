// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MultiDeviceWidget from './MultiDeviceWidget';

const fixtures = vi.hoisted(() => ({
  items: [
    { kind: 'device', key: 'device-1', commandId: 'device-1', device: { displayName: 'Desk lamp', state: { on: true } } },
    { kind: 'group', key: 'group-1', id: 'group-1', group: { name: 'Living room' }, mixed: true },
  ],
}));

vi.mock('../../../../context/AuthContext', () => ({
  useAuth: () => ({ user: { role: 'resident' }, accessToken: 'token', bootstrapping: false }),
}));
vi.mock('../../../../hooks/useDeviceHubDevices', () => ({
  default: () => ({ devices: [], loading: false, connectionInfo: { commandsReady: true } }),
}));
vi.mock('../../../../hooks/useErsInventory', () => ({
  default: () => ({ devices: [], groups: [], loading: false }),
}));
vi.mock('../../../../utils/groupControls', async (importOriginal) => ({
  ...await importOriginal(),
  canToggleDevice: () => true,
}));
vi.mock('../../../../utils/quickControls', () => ({
  resolveQuickControlItems: () => fixtures.items,
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('MultiDeviceWidget', () => {
  let root;
  let container;

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
    fixtures.items.splice(2);
  });

  it('shows device and group status beside the icon and the name below', () => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<MultiDeviceWidget settings={{ device_ids: ['device-1'], group_ids: ['group-1'] }} />));

    const tiles = container.querySelectorAll('.multi-device-widget__tile');
    expect(tiles).toHaveLength(2);
    for (const [index, [name, status]] of [['Desk lamp', 'On'], ['Living room', 'Mixed']].entries()) {
      const tile = tiles[index];
      const indicator = tile.querySelector('.multi-device-widget__tile-indicator');
      expect(indicator.querySelector('.multi-device-widget__tile-icon')).not.toBeNull();
      expect(indicator.querySelector('.multi-device-widget__tile-status').textContent).toBe(status);
      expect(indicator.nextElementSibling.textContent).toBe(name);
      expect(indicator.nextElementSibling.title).toBe(name);
    }
  });

  it('keeps multiple controls in a scrollable widget content area', () => {
    fixtures.items.push(...Array.from({ length: 4 }, (_, index) => ({
      kind: 'device',
      key: `extra-${index}`,
      commandId: `extra-${index}`,
      device: { displayName: `Living room device ${index}`, state: { on: false } },
    })));
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<MultiDeviceWidget settings={{ device_ids: ['device-1'], group_ids: ['group-1'] }} />));

    const content = container.querySelector('.widget-shell .widget-shell__content .multi-device-widget__content');
    expect(content.querySelectorAll('.multi-device-widget__tile')).toHaveLength(6);

    const style = document.createElement('style');
    style.textContent = readFileSync('src/components/Home/Dashboard/widgets/MultiDeviceWidget.css', 'utf8');
    document.head.appendChild(style);
    try {
      const rule = Array.from(style.sheet.cssRules).find(
        (cssRule) => cssRule.selectorText === '.multi-device-widget__content',
      );
      expect(rule?.style.getPropertyValue('overflow-y')).toBe('auto');
      expect(Number.parseFloat(rule?.style.getPropertyValue('min-height'))).toBe(0);
      expect(rule?.style.getPropertyValue('scroll-padding')).toBe('4px');
      const gridRule = Array.from(style.sheet.cssRules).find(
        (cssRule) => cssRule.selectorText === '.multi-device-widget__grid',
      );
      expect(gridRule?.style.getPropertyValue('grid-auto-rows')).toBe('minmax(64px, auto)');
      expect(gridRule?.style.getPropertyValue('gap')).toBe('0.6rem');
    } finally {
      style.remove();
    }
  });
});