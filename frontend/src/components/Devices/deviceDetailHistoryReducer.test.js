import { describe, expect, it } from 'vitest';
import {
  deviceDetailHistoryInitialState,
  deviceDetailHistoryReducer,
} from './deviceDetailHistoryReducer';

describe('deviceDetailHistoryReducer', () => {
  it('updates history query controls with set-field and update-field', () => {
    const withPreset = deviceDetailHistoryReducer(deviceDetailHistoryInitialState, {
      type: 'set-field',
      key: 'rangePreset',
      value: '7d',
    });
    const withLimit = deviceDetailHistoryReducer(withPreset, {
      type: 'update-field',
      key: 'limit',
      updater: (prev) => prev + 10,
    });

    expect(withPreset.rangePreset).toBe('7d');
    expect(withLimit.limit).toBe(310);
  });

  it('tracks loading and points transitions', () => {
    const loading = deviceDetailHistoryReducer(deviceDetailHistoryInitialState, {
      type: 'set-field',
      key: 'historyLoading',
      value: true,
    });
    const withPoints = deviceDetailHistoryReducer(loading, {
      type: 'set-field',
      key: 'historyPoints',
      value: [{ ts: '1', payload: {} }],
    });
    const done = deviceDetailHistoryReducer(withPoints, {
      type: 'set-field',
      key: 'historyLoading',
      value: false,
    });

    expect(loading.historyLoading).toBe(true);
    expect(withPoints.historyPoints).toHaveLength(1);
    expect(done.historyLoading).toBe(false);
  });

  it('handles overlay open and close transitions', () => {
    const opening = deviceDetailHistoryReducer(deviceDetailHistoryInitialState, {
      type: 'set-overlay',
      overlay: { key: 'temperature', fromRect: { left: 1 } },
      overlayPhase: 'opening',
    });
    const open = deviceDetailHistoryReducer(opening, {
      type: 'set-field',
      key: 'overlayPhase',
      value: 'open',
    });
    const cleared = deviceDetailHistoryReducer(open, { type: 'clear-overlay' });

    expect(opening.overlay?.key).toBe('temperature');
    expect(open.overlayPhase).toBe('open');
    expect(cleared.overlay).toBeNull();
    expect(cleared.overlayPhase).toBe('');
  });
});
