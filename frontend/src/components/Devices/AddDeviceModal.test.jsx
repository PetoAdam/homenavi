/** @vitest-environment jsdom */

import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AddDeviceModal from './AddDeviceModal';

const matterPairingProfile = {
  protocol: 'matter',
  label: 'Matter',
  schema_version: '1.0',
  supported: true,
  supports_interview: true,
  default_timeout_sec: 300,
  cta_label: 'Start Matter pairing',
  notes: 'Follow the guided steps below to start pairing.',
  flow: {
    id: 'matter-commissioning-v1',
    entry_modes: ['qr_code', 'manual_code', 'on_network'],
    forms: [
      {
        mode: 'qr_code',
        label: 'QR Code',
        fields: [
          { id: 'onboarding_payload', component: 'qr_payload', label: 'Onboarding payload', required: true },
        ],
      },
      {
        mode: 'manual_code',
        label: 'Manual Code',
        fields: [
          { id: 'manual_code', component: 'text', label: 'Manual setup code', required: true, placeholder: '12345678' },
          {
            id: 'network_path',
            component: 'select',
            label: 'Network path',
            default: 'thread',
            options: [
              { value: 'thread', label: 'Thread' },
              { value: 'on_network', label: 'Direct IP / On-network' },
            ],
          },
          { id: 'thread_operational_dataset', component: 'text', label: 'Thread operational dataset', placeholder: 'hex:...' },
        ],
      },
      {
        mode: 'on_network',
        label: 'On-network',
        fields: [
          { id: 'manual_code', component: 'text', label: 'Setup code', required: true, placeholder: '12345678' },
          { id: 'discriminator', component: 'number', label: 'Discriminator', required: true },
          {
            id: 'commissioning_interface',
            component: 'text',
            label: 'Interface',
            placeholder: 'auto',
          },
        ],
      },
    ],
    steps: [
      { id: 'discovery', label: 'Discovery', stage: 'discovery' },
      { id: 'commissioning_complete', label: 'Commissioning Complete', stage: 'completed' },
    ],
  },
};

function renderModal(props = {}) {
  const onStartPairing = props.onStartPairing || vi.fn().mockResolvedValue({
    id: 'session-1',
    protocol: 'matter',
    status: 'active',
    started_at: '2026-01-01T12:00:00.000Z',
    expires_at: '2026-01-01T12:05:00.000Z',
  });

  const view = render(
    <AddDeviceModal
      open
      onClose={vi.fn()}
      integrations={[{ protocol: 'matter', label: 'Matter', status: 'active' }]}
      pairingConfig={{ matter: matterPairingProfile }}
      pairingSessions={{}}
      onStartPairing={onStartPairing}
      onStopPairing={vi.fn().mockResolvedValue(undefined)}
      {...props}
    />,
  );

  return { ...view, onStartPairing };
}

async function startManualCodePairing(user) {
  await user.click(screen.getByRole('button', { name: /continue to pairing flow/i }));
  await user.click(screen.getByRole('button', { name: /manual code/i }));
  await user.click(screen.getByRole('button', { name: /^continue$/i }));
  await user.type(screen.getByLabelText(/manual setup code/i), '12345678');
  await user.click(screen.getByRole('button', { name: /^continue$/i }));
  await user.click(screen.getByRole('button', { name: /start matter pairing/i }));
}

async function openOnNetworkPairing(user) {
  await user.click(screen.getByRole('button', { name: /continue to pairing flow/i }));
  await user.click(screen.getByRole('button', { name: /^on-network$/i }));
  await user.click(screen.getByRole('button', { name: /^continue$/i }));
}

function buildFailedMatterSession(overrides = {}) {
  return {
    id: 'session-1',
    protocol: 'matter',
    status: 'failed',
    stage: 'network_provisioning',
    active: false,
    mode: 'manual_code',
    flowId: 'matter-commissioning-v1',
    message: 'Thread dataset is required',
    errorCode: 'THREAD_DATASET_MISSING',
    requiredInputs: ['thread_operational_dataset'],
    inputs: {
      manual_code: '12345678',
      network_path: 'thread',
    },
    progress: {
      inputs: {
        manual_code: '12345678',
        network_path: 'thread',
      },
      error_code: 'THREAD_DATASET_MISSING',
    },
    startedAt: '2026-01-01T12:00:00.000Z',
    expiresAt: '2026-01-01T12:05:00.000Z',
    ...overrides,
  };
}

beforeEach(() => {
  const modalRoot = document.createElement('div');
  modalRoot.id = 'modal-root';
  document.body.appendChild(modalRoot);
});

afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
});

describe('AddDeviceModal terminal recovery', () => {
  it('requires discriminator for on-network mode before start', async () => {
    const user = userEvent.setup();
    const onStartPairing = vi.fn().mockResolvedValue({
      id: 'session-1',
      protocol: 'matter',
      status: 'active',
      started_at: '2026-01-01T12:00:00.000Z',
      expires_at: '2026-01-01T12:05:00.000Z',
    });

    renderModal({ onStartPairing });

    await openOnNetworkPairing(user);
    await user.type(screen.getByLabelText(/setup code/i), '12345678');
    expect(screen.getByRole('button', { name: /^continue$/i }).hasAttribute('disabled')).toBe(true);
    expect(onStartPairing).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText(/discriminator/i), '3840');
    await user.click(screen.getByRole('button', { name: /^continue$/i }));
    await user.click(screen.getByRole('button', { name: /start matter pairing/i }));

    expect(onStartPairing).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'on_network',
      inputs: expect.objectContaining({
        manual_code: '12345678',
        discriminator: 3840,
      }),
    }));
  });

  it('retries Matter failures with schema-derived fallback payloads', async () => {
    const user = userEvent.setup();
    const { rerender, container, onStartPairing } = renderModal();

    await startManualCodePairing(user);

    expect(onStartPairing).toHaveBeenCalledTimes(1);
    expect(onStartPairing).toHaveBeenLastCalledWith(expect.objectContaining({
      protocol: 'matter',
      mode: 'manual_code',
      flow_id: 'matter-commissioning-v1',
      inputs: expect.objectContaining({
        manual_code: '12345678',
        network_path: 'thread',
      }),
    }));

    rerender(
      <AddDeviceModal
        open
        onClose={vi.fn()}
        integrations={[{ protocol: 'matter', label: 'Matter', status: 'active' }]}
        pairingConfig={{ matter: matterPairingProfile }}
        pairingSessions={{ matter: buildFailedMatterSession() }}
        onStartPairing={onStartPairing}
        onStopPairing={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(await screen.findByText(/thread dataset input is required or invalid/i)).toBeTruthy();

    const switchModeButton = await screen.findByRole('button', { name: /switch network mode/i });
    const terminalActions = switchModeButton.closest('.add-device-pairing-terminal-actions');
    const buttonLabels = Array.from(terminalActions.querySelectorAll('button')).map(button => button.textContent?.trim());
    expect(buttonLabels).toEqual([
      'Switch network mode',
      'Retry with updated dataset',
      'Return to details',
    ]);

    await user.click(within(terminalActions).getByRole('button', { name: /retry with updated dataset/i }));

    expect(onStartPairing).toHaveBeenCalledTimes(2);
    expect(onStartPairing).toHaveBeenLastCalledWith(expect.objectContaining({
      protocol: 'matter',
      mode: 'manual_code',
      flow_id: 'matter-commissioning-v1',
      inputs: expect.objectContaining({
        manual_code: '12345678',
        network_path: 'on_network',
      }),
    }));
  });

  it('switches back into the wizard with an alternate mode after terminal failure', async () => {
    const user = userEvent.setup();
    const { rerender } = renderModal();

    await startManualCodePairing(user);

    rerender(
      <AddDeviceModal
        open
        onClose={vi.fn()}
        integrations={[{ protocol: 'matter', label: 'Matter', status: 'active' }]}
        pairingConfig={{ matter: matterPairingProfile }}
        pairingSessions={{ matter: buildFailedMatterSession() }}
        onStartPairing={vi.fn().mockResolvedValue({
          id: 'session-2',
          protocol: 'matter',
          status: 'active',
          started_at: '2026-01-01T12:06:00.000Z',
          expires_at: '2026-01-01T12:11:00.000Z',
        })}
        onStopPairing={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(await screen.findByRole('button', { name: /switch network mode/i })).toBeTruthy();

    await user.click(screen.getByRole('button', { name: /switch network mode/i }));

    expect(screen.queryByText(/thread dataset input is required or invalid/i)).toBeNull();
    expect(screen.getByText(/provide missing inputs/i)).toBeTruthy();
    expect(screen.getByLabelText(/onboarding payload/i)).toBeTruthy();
    expect(screen.queryByLabelText(/pairing mode/i)).toBeNull();
  });

  it('stops an active pairing session from the progress panel', async () => {
    const user = userEvent.setup();
    const onStopPairing = vi.fn().mockResolvedValue(undefined);

    render(
      <AddDeviceModal
        open
        onClose={vi.fn()}
        integrations={[{ protocol: 'matter', label: 'Matter', status: 'active' }]}
        pairingConfig={{ matter: matterPairingProfile }}
        pairingSessions={{
          matter: {
            id: 'session-live',
            protocol: 'matter',
            status: 'in_progress',
            stage: 'discovery',
            active: true,
            mode: 'manual_code',
            startedAt: '2026-01-01T12:00:00.000Z',
            expiresAt: '2026-01-01T12:05:00.000Z',
            progress: {},
          },
        }}
        onStartPairing={vi.fn()}
        onStopPairing={onStopPairing}
      />,
    );

    await user.click(await screen.findByRole('button', { name: /stop pairing/i }));

    expect(onStopPairing).toHaveBeenCalledWith('matter');
  });
});