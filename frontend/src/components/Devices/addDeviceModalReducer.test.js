import { describe, expect, it } from 'vitest';
import {
  addDeviceModalReducer,
  createAddDeviceModalInitialState,
  createDefaultAddDeviceForm,
} from './addDeviceModalReducer';

describe('addDeviceModalReducer', () => {
  it('resets flow state while preserving form values', () => {
    const base = createAddDeviceModalInitialState();
    const withFormAndFlow = {
      ...base,
      form: {
        ...base.form,
        protocol: 'zigbee',
        name: 'Desk Sensor',
      },
      showAdvanced: true,
      flowStep: 'pairing',
      pairingError: 'failed',
      pairingNotice: 'waiting',
      pairingSetupOverride: true,
      pairedDevicesSummary: [{ deviceId: 'dev-1' }],
    };

    const next = addDeviceModalReducer(withFormAndFlow, { type: 'reset-flow' });

    expect(next.form.protocol).toBe('zigbee');
    expect(next.form.name).toBe('Desk Sensor');
    expect(next.showAdvanced).toBe(false);
    expect(next.flowStep).toBe('form');
    expect(next.pairingError).toBeNull();
    expect(next.pairingNotice).toBe('');
    expect(next.pairingSetupOverride).toBe(false);
    expect(next.pairedDevicesSummary).toEqual([]);
  });

  it('resets all state including form defaults', () => {
    const base = createAddDeviceModalInitialState();
    const dirty = {
      ...base,
      form: { ...base.form, protocol: 'matter', icon: 'sensor' },
      flowStep: 'success',
      pairingNotice: 'done',
    };

    const reset = addDeviceModalReducer(dirty, { type: 'reset-all' });

    expect(reset).toEqual(createAddDeviceModalInitialState());
    expect(reset.form).toEqual(createDefaultAddDeviceForm());
  });

  it('updates form values through updater and applies patch fields', () => {
    const base = createAddDeviceModalInitialState();
    const updatedForm = addDeviceModalReducer(base, {
      type: 'update-form',
      updater: (prev) => ({ ...prev, protocol: 'mock', model: 'X1' }),
    });
    const patched = addDeviceModalReducer(updatedForm, {
      type: 'patch',
      partial: {
        flowStep: 'pairing',
        pairingStartPending: true,
      },
    });

    expect(updatedForm.form.protocol).toBe('mock');
    expect(updatedForm.form.model).toBe('X1');
    expect(patched.flowStep).toBe('pairing');
    expect(patched.pairingStartPending).toBe(true);
  });
});
