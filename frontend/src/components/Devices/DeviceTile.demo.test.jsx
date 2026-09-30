// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DeviceTile from './DeviceTile';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderIntoDom(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('DeviceTile public demo guardrails', () => {
  const originalDemoMode = import.meta.env.VITE_DEMO_MODE;

  beforeEach(() => {
    import.meta.env.VITE_DEMO_MODE = '1';
  });

  afterEach(() => {
    document.body.innerHTML = '';
    vi.clearAllMocks();
    if (typeof originalDemoMode === 'undefined') {
      delete import.meta.env.VITE_DEMO_MODE;
    } else {
      import.meta.env.VITE_DEMO_MODE = originalDemoMode;
    }
  });

  it('blocks device deletion after the confirmation click and shows snackbar feedback', async () => {
    const onDelete = vi.fn().mockResolvedValue(undefined);
    const view = renderIntoDom(
      <DeviceTile
        device={{ id: 'device-1', displayName: 'Kitchen Lamp', online: true, protocol: 'zigbee', state: {}, capabilities: [] }}
        onDelete={onDelete}
      />
    );

    const openMenuButton = Array.from(document.querySelectorAll('button')).find((entry) => entry.getAttribute('title') === 'device -> Edit');
    act(() => {
      openMenuButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const openDeleteButton = Array.from(document.querySelectorAll('button')).find((entry) => entry.textContent?.trim() === 'Delete');
    act(() => {
      openDeleteButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const confirmButton = Array.from(document.querySelectorAll('button')).find((entry) => entry.className.includes('device-delete-confirm'));
    act(() => {
      confirmButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain('This action is disabled in the public demo');
    expect(onDelete).not.toHaveBeenCalled();

    view.unmount();
  });
});