// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MapDevicePalette from './MapDevicePalette';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('MapDevicePalette', () => {
  let root;
  let container;

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
    root = null;
    container = null;
  });

  it('allows full device names to wrap on touch devices', () => {
    const style = document.createElement('style');
    style.textContent = readFileSync('src/components/Map/Map.css', 'utf8');
    document.head.appendChild(style);
    try {
      const touchRules = Array.from(style.sheet.cssRules).find(
        (rule) => rule.conditionText === '(hover: none), (any-pointer: coarse)',
      );
      const labelRule = Array.from(touchRules?.cssRules || []).find(
        (rule) => rule.selectorText === '.map-palette-item .label span',
      );
      expect(labelRule?.style.getPropertyValue('white-space')).toBe('normal');
      expect(labelRule?.style.getPropertyValue('overflow-wrap')).toBe('anywhere');
    } finally {
      style.remove();
    }
  });

  it('retains the full device name and supports selecting and dragging it', () => {
    const setSelectedDeviceId = vi.fn();
    const name = 'A very long device name that needs to be truncated';
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(
      <MapDevicePalette
        devices={[{ id: 'device-1', displayName: name }]}
        busy={false}
        selectedDeviceId=""
        setSelectedDeviceId={setSelectedDeviceId}
      />,
    ));

    const item = container.querySelector('.map-palette-item');
    expect(item.textContent).toBe(name);
    expect(item.title).toContain(name);
    act(() => item.click());
    expect(setSelectedDeviceId.mock.calls[0][0]('')).toBe('device-1');
    expect(setSelectedDeviceId.mock.calls[0][0]('device-1')).toBe('');

    const setData = vi.fn();
    const event = new Event('dragstart', { bubbles: true });
    Object.defineProperty(event, 'dataTransfer', { value: { setData, effectAllowed: '' } });
    act(() => item.dispatchEvent(event));
    expect(setData).toHaveBeenCalledWith('text/homenavi-device-key', 'device-1');
    expect(event.dataTransfer.effectAllowed).toBe('move');
  });
});