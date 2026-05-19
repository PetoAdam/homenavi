import { describe, it, expect } from 'vitest';
import {
  buildPairingProgressSession,
  mergeMetadataRecord,
  mergeStateRecord,
  pairingConfigArrayToMap,
} from './useDeviceHubDevices.js';

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

  it('normalizes completed pairing progress events and preserves the active session id when the event omits it', () => {
    const session = buildPairingProgressSession({
      origin: 'device-hub',
      stage: 'completed',
      status: 'successful',
      active: false,
    }, 'zigbee', {
      id: 'pairing-session-1',
      protocol: 'zigbee',
      active: true,
      status: 'interview_complete',
      metadata: { type: 'light' },
    });

    expect(session).toMatchObject({
      id: 'pairing-session-1',
      protocol: 'zigbee',
      status: 'completed',
      stage: 'completed',
      active: false,
      metadata: { type: 'light' },
    });
  });

  it('keeps the most advanced non-terminal pairing step when later events arrive out of order', () => {
    const interviewing = buildPairingProgressSession({
      id: 'pairing-session-1',
      origin: 'device-hub',
      stage: 'interviewing',
      status: 'started',
      active: true,
    }, 'zigbee', {
      id: 'pairing-session-1',
      protocol: 'zigbee',
      active: true,
      status: 'device_joined',
      stage: 'device_joined',
    });

    const regressed = buildPairingProgressSession({
      id: 'pairing-session-1',
      origin: 'device-hub',
      stage: 'device_detected',
      status: '',
      active: true,
    }, 'zigbee', interviewing);

    expect(regressed).toMatchObject({
      id: 'pairing-session-1',
      protocol: 'zigbee',
      active: true,
      stage: 'interviewing',
      status: 'interviewing',
    });
  });

  it('uses the stage as the canonical runtime status for interview milestones', () => {
    const session = buildPairingProgressSession({
      id: 'pairing-session-1',
      origin: 'device-hub',
      stage: 'interview_complete',
      status: 'successful',
      active: true,
    }, 'zigbee');

    expect(session).toMatchObject({
      stage: 'interview_complete',
      status: 'interview_complete',
      active: true,
    });
  });

  it('normalizes multi-device pairing metadata from realtime events', () => {
    const session = buildPairingProgressSession({
      id: 'pairing-session-2',
      origin: 'device-hub',
      stage: 'device_added',
      status: 'device_added',
      active: true,
      allow_multiple_devices: true,
      added_devices: [
        {
          device_id: 'zigbee/0x00124b0024abcd01',
          protocol: 'zigbee',
          external_id: '0x00124b0024abcd01',
          state: 'completed',
          model: 'Hue White',
          added_at: '2026-05-19T10:00:00Z',
        },
      ],
    }, 'zigbee');

    expect(session).toMatchObject({
      allowMultipleDevices: true,
      active: true,
      status: 'device_added',
      stage: 'device_added',
    });
    expect(session.addedDevices).toHaveLength(1);
    expect(session.addedDevices[0]).toMatchObject({
      deviceId: 'zigbee/0x00124b0024abcd01',
      externalId: '0x00124b0024abcd01',
      state: 'completed',
      model: 'Hue White',
    });
  });

  it('keeps placeholder multi-device entries before a canonical device id exists', () => {
    const session = buildPairingProgressSession({
      id: 'pairing-session-3',
      origin: 'device-hub',
      stage: 'device_detected',
      status: 'device_detected',
      active: true,
      allow_multiple_devices: true,
      added_devices: [
        {
          protocol: 'zigbee',
          external_id: '0x00124b0024abcd02',
          state: 'detected',
        },
      ],
    }, 'zigbee');

    expect(session.addedDevices).toHaveLength(1);
    expect(session.addedDevices[0]).toMatchObject({
      deviceId: '',
      externalId: '0x00124b0024abcd02',
      state: 'detected',
    });
  });
});
