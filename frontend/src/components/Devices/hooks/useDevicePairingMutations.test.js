import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../../services/deviceHubService', () => ({
  listPairings: vi.fn(),
  startPairing: vi.fn(),
  stopPairing: vi.fn(),
}));

import {
  listPairings,
  startPairing,
  stopPairing,
} from '../../../services/deviceHubService';
import {
  findPairingSessionByProtocol,
  reconcileStartPairing,
  reconcileStopPairing,
} from './useDevicePairingMutations';

describe('findPairingSessionByProtocol', () => {
  it('matches protocols case-insensitively and can require active sessions', () => {
    const sessions = [
      { protocol: 'zigbee', active: false, id: '1' },
      { protocol: 'Matter', active: true, id: '2' },
    ];

    expect(findPairingSessionByProtocol(sessions, 'matter', { activeOnly: true })).toEqual({ protocol: 'Matter', active: true, id: '2' });
    expect(findPairingSessionByProtocol(sessions, 'zigbee', { activeOnly: true })).toBeNull();
  });
});

describe('reconcileStartPairing', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('reuses an existing active session before starting a new one', async () => {
    const refreshPairings = vi.fn().mockResolvedValue(undefined);
    listPairings.mockResolvedValue({ success: true, data: [{ protocol: 'zigbee', active: true, id: 'existing' }] });

    await expect(reconcileStartPairing({
      payload: { protocol: 'zigbee' },
      accessToken: 'token',
      refreshPairings,
    })).resolves.toEqual({ protocol: 'zigbee', active: true, id: 'existing' });

    expect(startPairing).not.toHaveBeenCalled();
    expect(refreshPairings).toHaveBeenCalledTimes(1);
  });

  it('reconciles a 409 conflict by reloading the current session', async () => {
    const refreshPairings = vi.fn().mockResolvedValue(undefined);
    listPairings
      .mockResolvedValueOnce({ success: true, data: [] })
      .mockResolvedValueOnce({ success: true, data: [{ protocol: 'matter', active: true, id: 'conflict-session' }] });
    startPairing.mockResolvedValue({ success: false, status: 409, error: 'Conflict' });

    await expect(reconcileStartPairing({
      payload: { protocol: 'matter' },
      accessToken: 'token',
      refreshPairings,
    })).resolves.toEqual({ protocol: 'matter', active: true, id: 'conflict-session' });

    expect(startPairing).toHaveBeenCalledTimes(1);
    expect(refreshPairings).toHaveBeenCalledTimes(1);
  });
});

describe('reconcileStopPairing', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('stops pairing and refreshes the live pairing state', async () => {
    const refreshPairings = vi.fn().mockResolvedValue(undefined);
    stopPairing.mockResolvedValue({ success: true, data: { stopped: true } });

    await expect(reconcileStopPairing({
      protocol: 'zigbee',
      accessToken: 'token',
      refreshPairings,
    })).resolves.toEqual({ stopped: true });

    expect(stopPairing).toHaveBeenCalledWith('zigbee', 'token');
    expect(refreshPairings).toHaveBeenCalledTimes(1);
  });
});
