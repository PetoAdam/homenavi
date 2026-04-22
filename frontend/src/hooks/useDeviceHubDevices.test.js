import { describe, it, expect } from 'vitest';
import { mergeMetadataRecord, mergeStateRecord, pairingConfigArrayToMap } from './useDeviceHubDevices.js';

describe('pairingConfigArrayToMap', () => {
  it('returns empty object for non-array payloads', () => {
    expect(pairingConfigArrayToMap(null)).toEqual({});
    expect(pairingConfigArrayToMap({})).toEqual({});
    expect(pairingConfigArrayToMap('nope')).toEqual({});
  });

  it('maps protocols to lowercase keys and preserves objects', () => {
    const payload = [
      { protocol: 'ZigBee', label: 'ZB', supported: true },
      { protocol: 'THREAD', label: 'Thread', supported: false },
    ];
    const result = pairingConfigArrayToMap(payload);
    expect(result).toHaveProperty('zigbee');
    expect(result).toHaveProperty('thread');
    expect(result.zigbee.label).toBe('ZB');
    expect(result.thread.supported).toBe(false);
  });

  it('ignores entries without a protocol string', () => {
    const payload = [
      { protocol: 'matter', label: 'Matter' },
      { label: 'no protocol' },
      null,
      undefined,
    ];
    const result = pairingConfigArrayToMap(payload);
    expect(Object.keys(result)).toEqual(['matter']);
  });
});

describe('useDeviceHubDevices realtime merge helpers', () => {
  it('caches zigbee state until metadata arrives, then applies it', () => {
    const cached = mergeStateRecord({}, 'zigbee/0xa4c13867e32d96d4', {
      device_id: 'zigbee/0xa4c13867e32d96d4',
      ts: 100,
      corr: 'corr-1',
      state: { state: false, brightness: 50 },
    });

    expect(cached.__pendingState).toEqual({ state: false, brightness: 50 });
    expect(cached._last_state).toBeUndefined();

    const merged = mergeMetadataRecord(cached, {
      device_id: 'zigbee/0xa4c13867e32d96d4',
      protocol: 'zigbee',
      manufacturer: 'Test',
      capabilities: [{ id: 'state' }],
    }, 'zigbee/0xa4c13867e32d96d4', 'zigbee');

    expect(merged.__hasMetadata).toBe(true);
    expect(merged._last_state).toEqual({ state: false, brightness: 50 });
    expect(merged.__pendingState).toBeUndefined();
    expect(merged.__lastStateCorr).toBe('corr-1');
  });

  it('drops stale state updates that are older than the latest applied state', () => {
    const prev = {
      __hasMetadata: true,
      stateUpdatedAt: 200,
      _last_state: { state: true },
    };

    const merged = mergeStateRecord(prev, 'zigbee/0xb4e84287377c0000', {
      device_id: 'zigbee/0xb4e84287377c0000',
      ts: 150,
      state: { state: false },
    });

    expect(merged).toBeNull();
  });

  it('preserves the previous live state when a REST snapshot arrives without state values', () => {
    const prev = {
      __hasMetadata: true,
      _last_state: { state: true, brightness: 120 },
      stateUpdatedAt: 200,
      online: false,
    };

    const merged = mergeMetadataRecord(prev, {
      device_id: 'zigbee/0xa4c13867e32d96d4',
      protocol: 'zigbee',
      manufacturer: 'Test',
      online: true,
    }, 'zigbee/0xa4c13867e32d96d4', 'zigbee');

    expect(merged._last_state).toEqual({ state: true, brightness: 120 });
    expect(merged.online).toBe(true);
  });
});
