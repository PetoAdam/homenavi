import { describe, expect, it } from 'vitest';

import { resolveCommandDeviceId } from './deviceIdentity';

describe('resolveCommandDeviceId', () => {
  it('prefers the canonical HDP id over ERS ids', () => {
    expect(resolveCommandDeviceId({
      id: 'ers-device-123',
      ersId: 'ers-device-123',
      hdpId: 'zigbee/0x00124b0024abcd01',
    })).toBe('zigbee/0x00124b0024abcd01');
  });

  it('falls back to the first HDP id from hdpIds when hdpId is absent', () => {
    expect(resolveCommandDeviceId({
      id: 'ers-device-123',
      hdpIds: ['emodul/module-a/zone/1'],
    })).toBe('emodul/module-a/zone/1');
  });

  it('uses the database-backed device id only when no HDP id is available', () => {
    expect(resolveCommandDeviceId({
      id: '8f065d7c-4329-4abc-a8d6-fb1122334455',
    })).toBe('8f065d7c-4329-4abc-a8d6-fb1122334455');
  });
});