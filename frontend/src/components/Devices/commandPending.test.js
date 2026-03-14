import { describe, expect, it } from 'vitest';

import { shouldClearPendingFromDevice } from './commandPending';

describe('shouldClearPendingFromDevice', () => {
  it('clears pending when expected state is satisfied by a matching state correlation', () => {
    const pending = {
      corr: 'corr-123',
      startedAt: Date.now() - 5000,
      stateVersion: 100,
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      lastStateCorr: 'corr-123',
      stateUpdatedAt: 200,
      state: { state: true },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(true);
  });

  it('clears pending for changed state without command_result when state correlation matches', () => {
    const pending = {
      corr: 'corr-456',
      startedAt: Date.now() - 5000,
      stateVersion: 100,
      baselineState: { brightness: 10 },
    };

    const device = {
      lastStateCorr: 'corr-456',
      stateUpdatedAt: 200,
      state: { brightness: 50 },
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(true);
  });
});