import { describe, expect, it } from 'vitest';
import { mapControllerUiInitialState, mapControllerUiReducer } from './mapControllerUiReducer';

describe('mapControllerUiReducer', () => {
  it('updates primitive fields and supports updater callbacks', () => {
    const state = mapControllerUiReducer(mapControllerUiInitialState(), {
      type: 'set-field',
      key: 'mode',
      value: 'draw',
    });
    const next = mapControllerUiReducer(state, {
      type: 'set-field',
      key: 'labelScale',
      value: prev => prev + 0.25,
    });

    expect(state.mode).toBe('draw');
    expect(next.labelScale).toBe(1.25);
  });

  it('merges snap settings and resets the draft flow', () => {
    const withSnap = mapControllerUiReducer(mapControllerUiInitialState(), {
      type: 'merge-snap-settings',
      value: { grid: true },
    });
    const withDraft = mapControllerUiReducer(withSnap, {
      type: 'set-field',
      key: 'draft',
      value: { id: 'room-1' },
    });
    const reset = mapControllerUiReducer(withDraft, { type: 'reset-draft' });

    expect(withSnap.snapSettings.grid).toBe(true);
    expect(reset.draft).toBeNull();
    expect(reset.mode).toBe('select');
  });

  it('resets favorites editor independently', () => {
    const next = mapControllerUiReducer(mapControllerUiInitialState(), {
      type: 'set-field',
      key: 'favoritesEditorKey',
      value: 'device-1',
    });
    const reset = mapControllerUiReducer(next, { type: 'reset-favorites-editor' });

    expect(next.favoritesEditorKey).toBe('device-1');
    expect(reset.favoritesEditorKey).toBe('');
  });
});
