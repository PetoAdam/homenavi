import { describe, expect, it } from 'vitest';
import { deviceDetailUiInitialState, deviceDetailUiReducer } from './deviceDetailUiReducer';

describe('deviceDetailUiReducer', () => {
  it('sets known fields and ignores unknown keys', () => {
    const next = deviceDetailUiReducer(deviceDetailUiInitialState, {
      type: 'set-field',
      key: 'commandError',
      value: 'failed',
    });
    const ignored = deviceDetailUiReducer(next, {
      type: 'set-field',
      key: 'unknown',
      value: 'x',
    });

    expect(next.commandError).toBe('failed');
    expect(ignored).toBe(next);
  });

  it('updates fields via updater callbacks', () => {
    const withPending = deviceDetailUiReducer(deviceDetailUiInitialState, {
      type: 'set-field',
      key: 'pendingCommand',
      value: { corr: 'abc' },
    });
    const cleared = deviceDetailUiReducer(withPending, {
      type: 'update-field',
      key: 'pendingCommand',
      updater: (prev) => (prev?.corr === 'abc' ? null : prev),
    });

    expect(withPending.pendingCommand).toEqual({ corr: 'abc' });
    expect(cleared.pendingCommand).toBeNull();
  });

  it('supports edit tag array transitions and no-op updater', () => {
    const selected = deviceDetailUiReducer(deviceDetailUiInitialState, {
      type: 'set-field',
      key: 'editTagIds',
      value: ['t1'],
    });
    const appended = deviceDetailUiReducer(selected, {
      type: 'update-field',
      key: 'editTagIds',
      updater: (prev) => (prev.includes('t2') ? prev : [...prev, 't2']),
    });
    const noOp = deviceDetailUiReducer(appended, {
      type: 'update-field',
      key: 'editTagIds',
      updater: (prev) => prev,
    });

    expect(appended.editTagIds).toEqual(['t1', 't2']);
    expect(noOp).toBe(appended);
  });
});
