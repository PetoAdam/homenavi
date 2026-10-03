import { useCallback, useEffect, useRef, useState } from 'react';

export const INVENTORY_MUTATION_REFRESH_DELAY_MS = 350;

export function createInventoryMutationRefresh({
  refreshDeviceHub,
  refreshErs,
  onSettled,
  delayMs = INVENTORY_MUTATION_REFRESH_DELAY_MS,
  setTimeoutFn = setTimeout,
  clearTimeoutFn = clearTimeout,
}) {
  let timerId = null;
  let refreshPromise = null;

  const refresh = async () => {
    timerId = null;
    refreshPromise = Promise.all([
      Promise.resolve().then(() => refreshDeviceHub?.()),
      Promise.resolve().then(() => refreshErs?.()),
    ]);
    try {
      await refreshPromise;
      onSettled?.(null);
    } catch (error) {
      const message = error?.message || 'Unable to refresh device inventory';
      onSettled?.(message);
      throw error;
    } finally {
      refreshPromise = null;
    }
  };

  const schedule = () => {
    if (timerId !== null) clearTimeoutFn(timerId);
    timerId = setTimeoutFn(() => {
      void refresh().catch(() => {});
    }, delayMs);
  };

  const retry = () => {
    if (refreshPromise) return refreshPromise;
    return refresh();
  };

  const dispose = () => {
    if (timerId !== null) clearTimeoutFn(timerId);
    timerId = null;
  };

  return { schedule, retry, dispose };
}

export function useInventoryMutationRefresh({ refreshDeviceHub, refreshErs }) {
  const [error, setError] = useState('');
  const coordinatorRef = useRef(null);

  useEffect(() => {
    const coordinator = createInventoryMutationRefresh({
      refreshDeviceHub,
      refreshErs,
      onSettled: setError,
    });
    coordinatorRef.current = coordinator;
    return () => {
      coordinator.dispose();
      if (coordinatorRef.current === coordinator) coordinatorRef.current = null;
    };
  }, [refreshDeviceHub, refreshErs]);

  const notifyMutationSucceeded = useCallback(() => {
    coordinatorRef.current?.schedule();
  }, []);

  const retry = useCallback(async () => {
    if (!coordinatorRef.current) return;
    await coordinatorRef.current.retry();
  }, []);

  return { error, notifyMutationSucceeded, retry };
}