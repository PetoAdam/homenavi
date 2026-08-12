import { describe, expect, it } from 'vitest';
import { devicesUiInitialState, devicesUiReducer } from './devicesUiReducer';

describe('devicesUiReducer', () => {
  it('opens and closes add modal', () => {
    const open = devicesUiReducer(devicesUiInitialState, { type: 'open-add-modal' });
    const close = devicesUiReducer(open, { type: 'close-add-modal' });
    expect(open.showAddModal).toBe(true);
    expect(close.showAddModal).toBe(false);
  });

  it('sets and clears command error', () => {
    const withError = devicesUiReducer(devicesUiInitialState, {
      type: 'set-command-error',
      message: 'failed',
    });
    const cleared = devicesUiReducer(withError, { type: 'clear-command-error' });
    expect(withError.commandError).toBe('failed');
    expect(cleared.commandError).toBeNull();
  });

  it('sets and clears pending commands by id', () => {
    const queued = devicesUiReducer(devicesUiInitialState, {
      type: 'set-pending-command',
      id: 'device-1',
      pending: { corr: 'x' },
    });
    const cleared = devicesUiReducer(queued, {
      type: 'clear-pending-command',
      id: 'device-1',
    });

    expect(queued.pendingCommands['device-1']).toEqual({ corr: 'x' });
    expect(cleared.pendingCommands['device-1']).toBeUndefined();
  });

  it('supports pending command updater function', () => {
    const next = devicesUiReducer(devicesUiInitialState, {
      type: 'update-pending-commands',
      updater: (prev) => ({ ...prev, 'a': { corr: '1' } }),
    });
    expect(next.pendingCommands.a).toEqual({ corr: '1' });
  });
});
