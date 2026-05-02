import { describe, expect, it, vi } from 'vitest';

import {
  applyPendingStateToDevice,
  createPendingCommand,
  isTerminalCommandResult,
  shouldClearPendingFromDevice,
} from './commandPending';

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

  it('clears pending when a newer state echo matches the expected state even without a command result', () => {
    const pending = {
      corr: 'corr-state-only',
      stateVersion: 100,
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      state: { state: true },
      stateUpdatedAt: 200,
      lastCommandResult: null,
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(true);
  });

  it('keeps pending when the expected state already matched before any newer state arrived', () => {
    const pending = {
      corr: 'corr-no-advance',
      stateVersion: 200,
      baselineState: { state: false },
      expectedState: { state: true },
    };

    const device = {
      state: { state: true },
      stateUpdatedAt: 200,
      lastCommandResult: null,
    };

    expect(shouldClearPendingFromDevice(pending, device)).toBe(false);
  });

  it('clears pending when a correlated state echo changes from baseline without a command result', () => {
    const pending = {
      corr: 'corr-baseline-fallback',
      stateVersion: 100,
      baselineState: { brightness: 25 },
      expectedState: { level: 80 },
    };

    const device = {
      state: { brightness: 80 },
      stateUpdatedAt: 100,
      lastStateCorr: 'corr-baseline-fallback',
      lastCommandResult: null,
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

describe('command lifecycle helpers', () => {
  it('prefers the backend terminal flag when present', () => {
    expect(isTerminalCommandResult({ status: 'queued', terminal: true })).toBe(true);
    expect(isTerminalCommandResult({ status: 'applied', terminal: false })).toBe(false);
  });

  it('creates pending commands with a local timeout watchdog', () => {
    vi.useFakeTimers();
    const onTimeout = vi.fn();

    const { corr, enrichedPayload, pending } = createPendingCommand(
      { stateUpdatedAt: 100, state: { state: false } },
      { state: { state: true } },
      { timeoutMs: 2500, onTimeout },
    );

    expect(corr).toBeTruthy();
    expect(enrichedPayload.correlation_id).toBe(corr);
    expect(pending.expectedState).toEqual({ state: true });
    expect(pending.timeoutId).toBeTruthy();

    vi.advanceTimersByTime(2500);
    expect(onTimeout).toHaveBeenCalledTimes(1);
    expect(onTimeout.mock.calls[0][0].corr).toBe(corr);

    clearTimeout(pending.timeoutId);
    vi.useRealTimers();
  });
});