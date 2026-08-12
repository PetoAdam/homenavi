import { describe, expect, it } from 'vitest';
import {
  createDeviceHubConnectionInitialState,
  deviceHubConnectionReducer,
} from './deviceHubConnectionReducer';

describe('deviceHubConnectionReducer', () => {
  it('resets connection state for the requested metadata mode', () => {
    const next = deviceHubConnectionReducer(createDeviceHubConnectionInitialState('rest'), {
      type: 'reset',
      metadataMode: 'ws',
    });

    expect(next.metadataStatus).toEqual({ connected: false, source: 'ws' });
    expect(next.stateStatus).toEqual({ connected: false, subscribed: false, firstStateReceived: false });
  });

  it('updates status slices without replacing unrelated state', () => {
    const withMetadata = deviceHubConnectionReducer(createDeviceHubConnectionInitialState(), {
      type: 'set-metadata-status',
      value: { connected: true, source: 'rest' },
    });
    const withState = deviceHubConnectionReducer(withMetadata, {
      type: 'set-state-status',
      value: { connected: true, subscribed: true },
    });

    expect(withMetadata.metadataStatus.connected).toBe(true);
    expect(withState.stateStatus).toMatchObject({ connected: true, subscribed: true, firstStateReceived: false });
    expect(withState.metadataStatus.connected).toBe(true);
  });

  it('marks realtime metrics once and ignores repeats', () => {
    const initial = createDeviceHubConnectionInitialState();
    const withMetric = deviceHubConnectionReducer(initial, {
      type: 'mark-realtime-metric',
      key: 'authReadyMs',
      elapsed: 42,
    });
    const repeat = deviceHubConnectionReducer(withMetric, {
      type: 'mark-realtime-metric',
      key: 'authReadyMs',
      elapsed: 99,
    });

    expect(withMetric.realtimeMetrics.authReadyMs).toBe(42);
    expect(repeat).toBe(withMetric);
  });

  it('only flips firstStateReceived once', () => {
    const initial = createDeviceHubConnectionInitialState();
    const first = deviceHubConnectionReducer(initial, {
      type: 'set-first-state-received',
      value: true,
    });
    const second = deviceHubConnectionReducer(first, {
      type: 'set-first-state-received',
      value: false,
    });

    expect(first.stateStatus.firstStateReceived).toBe(true);
    expect(second).toBe(first);
  });
});
