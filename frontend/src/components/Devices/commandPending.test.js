import { describe, expect, it } from 'vitest';

import { applyPendingStateToDevice, shouldClearPendingFromDevice } from './commandPending';

describe('shouldClearPendingFromDevice', () => {
  it('clears pending when the device-hub emits applied and the live state matches the expectation', () => {
    const pending = {
      corr: 'corr-123',
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      state: { state: true },
      lastCommandResult: {
        corr: 'corr-123',
        origin: 'device-hub',
        status: 'applied',
      },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(true);
  });

  it('keeps pending when applied arrives before the state changes', () => {
    const pending = {
      corr: 'corr-stale',
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      state: { state: false },
      lastCommandResult: {
        corr: 'corr-stale',
        origin: 'device-hub',
        status: 'applied',
      },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(false);
  });

  it('still clears failed terminal states immediately', () => {
    const pending = {
      corr: 'corr-fail',
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      state: { state: false },
      lastCommandResult: {
        corr: 'corr-fail',
        origin: 'device-hub',
        status: 'failed',
      },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(true);
  });

  it('ignores non-hub command results even when the correlation matches', () => {
    const pending = {
      corr: 'corr-456',
    };

    const device = {
      lastCommandResult: {
        corr: 'corr-456',
        origin: 'adapter',
        status: 'failed',
      },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(false);
  });

  it('applies pending expected state locally for the initiating frontend', () => {
    const device = {
      state: { on: false, brightness: 20 },
      toggleState: false,
    };
    const pending = {
      corr: 'corr-789',
      expectedState: { on: true, brightness: 75 },
    };

    expect(applyPendingStateToDevice(device, pending)).toMatchObject({
      state: { on: true, brightness: 75 },
      toggleState: true,
      lastStateCorr: 'corr-789',
    });
  });
});