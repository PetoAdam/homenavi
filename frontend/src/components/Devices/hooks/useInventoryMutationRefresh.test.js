import { describe, expect, it, vi } from 'vitest';

import { createInventoryMutationRefresh } from './useInventoryMutationRefresh';

describe('createInventoryMutationRefresh', () => {
  it('coalesces multiple successful mutations into one trailing refresh', async () => {
    vi.useFakeTimers();
    const refreshDeviceHub = vi.fn().mockResolvedValue(undefined);
    const refreshErs = vi.fn().mockResolvedValue(undefined);
    const coordinator = createInventoryMutationRefresh({ refreshDeviceHub, refreshErs, delayMs: 350 });

    coordinator.schedule();
    coordinator.schedule();
    coordinator.schedule();
    await vi.advanceTimersByTimeAsync(350);

    expect(refreshDeviceHub).toHaveBeenCalledTimes(1);
    expect(refreshErs).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it('reports a refresh failure and retries both sources', async () => {
    const refreshDeviceHub = vi.fn()
      .mockRejectedValueOnce(new Error('Device Hub unavailable'))
      .mockResolvedValueOnce(undefined);
    const refreshErs = vi.fn().mockResolvedValue(undefined);
    const onSettled = vi.fn();
    const coordinator = createInventoryMutationRefresh({ refreshDeviceHub, refreshErs, onSettled });

    await expect(coordinator.retry()).rejects.toThrow('Device Hub unavailable');
    expect(onSettled).toHaveBeenCalledWith('Device Hub unavailable');

    await expect(coordinator.retry()).resolves.toBeUndefined();
    expect(onSettled).toHaveBeenLastCalledWith(null);
    expect(refreshDeviceHub).toHaveBeenCalledTimes(2);
    expect(refreshErs).toHaveBeenCalledTimes(2);
  });
});