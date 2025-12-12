import { describe, it, expect } from 'vitest';
import { pairingConfigArrayToMap } from './useDeviceHubDevices.js';

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
