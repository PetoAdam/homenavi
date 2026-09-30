// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import AddDeviceModal from './AddDeviceModal';

vi.mock('./pairing/pairingSchema', () => ({
  buildPairingStartPayload: vi.fn(),
  deriveRecoveryPreset: vi.fn(() => null),
  getBlockedReason: vi.fn(() => ''),
  getPairingProfile: vi.fn(() => ({ label: 'Zigbee', notes: 'Mock pairing notes' })),
  getProtocolOptions: vi.fn(() => [{ protocol: 'zigbee', label: 'Zigbee', status: 'connected' }]),
  isPairingSupported: vi.fn(() => true),
}));

vi.mock('./pairing/PairingFlowRenderer', () => ({
  default: () => <div>Pairing flow renderer</div>,
}));

vi.mock('./pairing/PairingProgressPanel', () => ({
  default: () => <div>Pairing progress panel</div>,
}));

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

describe('AddDeviceModal public demo guardrails', () => {
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

  it('blocks progression into the pairing flow and shows snackbar feedback', async () => {
    const view = renderIntoDom(
      <AddDeviceModal
        open
        onClose={vi.fn()}
        integrations={[{ id: 'zigbee', status: 'connected' }]}
        pairingSessions={{}}
        pairingConfig={{ zigbee: { label: 'Zigbee' } }}
        onStartPairing={vi.fn()}
        onStopPairing={vi.fn()}
      />
    );

    const continueButton = Array.from(document.querySelectorAll('button')).find((entry) => entry.textContent?.includes('Continue to pairing flow'));
    act(() => {
      continueButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain('This action is disabled in the public demo');
    expect(document.body.textContent).not.toContain('Pairing flow renderer');

    view.unmount();
  });
});