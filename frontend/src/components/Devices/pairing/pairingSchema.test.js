import { describe, expect, it } from 'vitest';
import {
  buildPairingPhaseStates,
  buildPairingStartPayload,
  defaultFieldValues,
  deriveRecoveryPreset,
  getFieldsForMode,
  getProtocolOptions,
  normalizePairingFlow,
  resolveCurrentPairingStep,
} from './pairingSchema';

const sampleFlow = {
  id: 'flow-1',
  entry_modes: ['default', 'qr_code'],
  forms: [
    {
      mode: 'default',
      fields: [
        { id: 'timeout', component: 'number', bind: 'timeout', default: 90 },
        { id: 'room_hint', component: 'text' },
      ],
    },
    {
      mode: 'qr_code',
      fields: [
        { id: 'onboarding_payload', component: 'qr_payload', required: true },
      ],
    },
  ],
  steps: [
    { id: 'active', label: 'Active', stage: 'active' },
    { id: 'completed', label: 'Completed', stage: 'completed' },
  ],
};

describe('normalizePairingFlow', () => {
  it('normalizes forms and entry modes', () => {
    const flow = normalizePairingFlow(sampleFlow);
    expect(flow.entryModes).toEqual(['default', 'qr_code']);
    expect(flow.forms).toHaveLength(2);
    expect(flow.forms[0].fields[0].bind).toBe('timeout');
    expect(flow.flowId).toBe('flow-1');
  });

  it('returns defaults for empty flow', () => {
    const flow = normalizePairingFlow(null);
    expect(flow.steps.length).toBeGreaterThan(0);
    expect(flow.entryModes).toEqual(['default']);
  });
});

describe('getFieldsForMode/defaultFieldValues', () => {
  it('returns fields for selected mode and defaults', () => {
    const fields = getFieldsForMode(sampleFlow, 'qr_code');
    expect(fields).toHaveLength(1);
    expect(fields[0].id).toBe('onboarding_payload');

    const values = defaultFieldValues(sampleFlow, 'default');
    expect(values.timeout).toBe(90);
    expect(values.room_hint).toBe('');
  });
});

describe('buildPairingPhaseStates', () => {
  it('marks active phase by status', () => {
    const phases = buildPairingPhaseStates('completed', sampleFlow);
    expect(phases[0].state).toBe('complete');
    expect(phases[1].state).toBe('active');
  });
});

describe('resolveCurrentPairingStep', () => {
  it('returns current step metadata from status', () => {
    const current = resolveCurrentPairingStep('completed', sampleFlow);
    expect(current.index).toBe(1);
    expect(current.step?.label).toBe('Completed');
    expect(current.total).toBe(2);
  });

  it('falls back to first step for unknown statuses', () => {
    const current = resolveCurrentPairingStep('unknown_status', sampleFlow);
    expect(current.index).toBe(0);
    expect(current.step?.label).toBe('Active');
  });
});

describe('buildPairingStartPayload', () => {
  it('extracts timeout and keeps schema inputs', () => {
    const payload = buildPairingStartPayload({
      protocol: 'mock',
      pairingProfile: { default_timeout_sec: 60, flow: sampleFlow },
      selectedMode: 'default',
      fieldValues: { timeout: 120, room_hint: 'Kitchen' },
    });

    expect(payload.protocol).toBe('mock');
    expect(payload.timeout).toBe(120);
    expect(payload.mode).toBe('default');
    expect(payload.flow_id).toBe('flow-1');
    expect(payload.inputs).toEqual({ room_hint: 'Kitchen' });
  });

  it('keeps required on-network discriminator when supplied', () => {
    const payload = buildPairingStartPayload({
      protocol: 'matter',
      pairingProfile: {
        default_timeout_sec: 300,
        flow: {
          id: 'matter-flow',
          entry_modes: ['on_network'],
          forms: [
            {
              mode: 'on_network',
              fields: [
                { id: 'manual_code', component: 'text', required: true },
                { id: 'discriminator', component: 'number', required: true },
              ],
            },
          ],
        },
      },
      selectedMode: 'on_network',
      fieldValues: { manual_code: '12345678', discriminator: 3840 },
    });

    expect(payload.mode).toBe('on_network');
    expect(payload.inputs).toEqual({ manual_code: '12345678', discriminator: 3840 });
  });
});

describe('getProtocolOptions', () => {
  it('includes only schema-backed supported protocols', () => {
    const options = getProtocolOptions(
      [
        { protocol: 'matter', label: 'Matter Adapter', status: 'active' },
        { protocol: 'connector', label: 'Connector', status: 'active' },
      ],
      {
        matter: { protocol: 'matter', schema_version: '1.0', supported: true, label: 'Matter' },
        connector: { protocol: 'connector', schema_version: '1.0', supported: false, label: 'Connector' },
        emodul: { protocol: 'emodul', supported: true, label: 'eModul' },
      },
    );

    expect(options).toHaveLength(1);
    expect(options[0].protocol).toBe('matter');
    expect(options[0].label).toBe('Matter Adapter');
  });
});

describe('deriveRecoveryPreset', () => {
  const matterFlow = {
    id: 'matter-flow',
    entry_modes: ['qr_code', 'manual_code', 'on_network'],
    forms: [
      {
        mode: 'qr_code',
        fields: [
          { id: 'onboarding_payload', component: 'qr_payload', required: true },
          { id: 'network_path', component: 'select' },
        ],
      },
      {
        mode: 'manual_code',
        fields: [
          { id: 'manual_code', component: 'text', required: true },
          { id: 'network_path', component: 'select' },
          { id: 'thread_operational_dataset', component: 'text' },
        ],
      },
      {
        mode: 'on_network',
        fields: [
          { id: 'manual_code', component: 'text', required: true },
          { id: 'network_path', component: 'select' },
        ],
      },
    ],
  };

  it('suggests on-network fallback for thread dataset failures', () => {
    const preset = deriveRecoveryPreset({
      pairingProfile: { flow: matterFlow },
      currentMode: 'manual_code',
      currentValues: { manual_code: '12345678', network_path: 'thread' },
      requiredFieldIds: [],
      terminalStatus: 'failed',
      errorCode: 'THREAD_DATASET_MISSING',
    });

    expect(preset.mode).toBe('on_network');
    expect(preset.values.network_path).toBe('on_network');
    expect(preset.values.manual_code).toBe('12345678');
  });

  it('prefers manual-capable mode for invalid manual code errors', () => {
    const preset = deriveRecoveryPreset({
      pairingProfile: { flow: matterFlow },
      currentMode: 'qr_code',
      currentValues: { onboarding_payload: 'MT:ABC' },
      requiredFieldIds: [],
      terminalStatus: 'failed',
      errorCode: 'MANUAL_CODE_INVALID',
    });

    expect(['manual_code', 'on_network']).toContain(preset.mode);
  });

  it('can switch to an alternate mode when requested', () => {
    const preset = deriveRecoveryPreset({
      pairingProfile: { flow: matterFlow },
      currentMode: 'manual_code',
      currentValues: { manual_code: '12345678' },
      requiredFieldIds: [],
      terminalStatus: 'stopped',
      errorCode: '',
      preferAlternateMode: true,
    });

    expect(preset.mode).not.toBe('manual_code');
    expect(['qr_code', 'on_network']).toContain(preset.mode);
  });
});
