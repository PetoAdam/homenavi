// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import DevicePresentationSettings from './DevicePresentationSettings';
import { patchErsDevice } from '../../services/entityRegistryService';

vi.mock('../../services/entityRegistryService', () => ({ patchErsDevice: vi.fn() }));
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('device presentation settings', () => {
  let root;
  let container;
  const device = {
    ersId: 'device-1',
    inputs: [{ id: 'target', label: 'Target', type: 'slider', range: { min: 0, max: 100 } }],
    capabilities: [{ id: 'target', name: 'Target', value_type: 'number', range: { min: 0, max: 100 }, access: { write: true } }, { id: 'details', value_type: 'object' }],
    meta: { presentation: { controls: { target: 'stepper' } } },
  };

  function render(props = {}) {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<DevicePresentationSettings device={device} accessToken="token" refresh={vi.fn()} {...props} />));
  }

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
    vi.resetAllMocks();
  });

  it('saves a compatible choice and resets to the integration default', async () => {
    const refresh = vi.fn();
    patchErsDevice.mockResolvedValue({ success: true });
    render({ refresh });
    const control = container.querySelector('[aria-label="Controls: Target"]');
    expect(control.value).toBe('stepper');
    expect(container.querySelector('[aria-label="State displays: Target"] option[value="gauge"]')).not.toBeNull();
    expect(container.querySelector('[aria-label="State displays: details"]')).toBeNull();
    await act(async () => { control.value = ''; control.dispatchEvent(new Event('change', { bubbles: true })); });
    expect(patchErsDevice).toHaveBeenCalledWith('device-1', { meta: { presentation: { controls: { target: null } } } }, 'token');
    expect(refresh).toHaveBeenCalledOnce();
  });

  it('reports a failed save', async () => {
    patchErsDevice.mockResolvedValue({ success: false, error: 'Save failed' });
    render();
    const display = container.querySelector('[aria-label="State displays: Target"]');
    await act(async () => { display.value = 'gauge'; display.dispatchEvent(new Event('change', { bubbles: true })); });
    expect(container.querySelector('[role="alert"]').textContent).toBe('Save failed');
  });
});