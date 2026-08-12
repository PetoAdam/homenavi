import { useMemo } from 'react';
import { useMutation } from '@tanstack/react-query';
import {
  listPairings as listPairingsApi,
  startPairing as startPairingApi,
  stopPairing as stopPairingApi,
} from '../../../services/deviceHubService';

function normalizeProtocol(value) {
  return typeof value === 'string' ? value.trim().toLowerCase() : '';
}

export function findPairingSessionByProtocol(sessions, protocol, { activeOnly = false } = {}) {
  const target = normalizeProtocol(protocol);
  if (!target || !Array.isArray(sessions)) return null;
  return sessions.find((item) => {
    const itemProtocol = normalizeProtocol(item?.protocol);
    if (itemProtocol !== target) return false;
    if (activeOnly && !item?.active) return false;
    return true;
  }) || null;
}

export async function reconcileStartPairing({ payload, accessToken, refreshPairings }) {
  if (!accessToken) {
    throw new Error('Authentication required');
  }

  const protocol = normalizeProtocol(payload?.protocol);
  if (protocol) {
    const listRes = await listPairingsApi(accessToken);
    if (listRes.success && Array.isArray(listRes.data)) {
      const activeSession = findPairingSessionByProtocol(listRes.data, protocol, { activeOnly: true });
      if (activeSession) {
        await refreshPairings?.();
        return activeSession;
      }
    }
  }

  const res = await startPairingApi(payload, accessToken);
  if (!res.success) {
    if (res.status === 409) {
      const listRes = await listPairingsApi(accessToken);
      if (listRes.success && Array.isArray(listRes.data)) {
        const match = findPairingSessionByProtocol(listRes.data, protocol);
        if (match) {
          await refreshPairings?.();
          return match;
        }
      }
    }
    throw new Error(res.error || 'Unable to start pairing');
  }

  await refreshPairings?.();
  return res.data;
}

export async function reconcileStopPairing({ protocol, accessToken, refreshPairings }) {
  if (!accessToken) {
    throw new Error('Authentication required');
  }

  const res = await stopPairingApi(protocol, accessToken);
  if (!res.success) {
    throw new Error(res.error || 'Unable to stop pairing');
  }

  await refreshPairings?.();
  return res.data;
}

export function startPairingMutationOptions({ accessToken, refreshPairings }) {
  return {
    mutationFn: async (payload) => reconcileStartPairing({ payload, accessToken, refreshPairings }),
  };
}

export function stopPairingMutationOptions({ accessToken, refreshPairings }) {
  return {
    mutationFn: async (protocol) => reconcileStopPairing({ protocol, accessToken, refreshPairings }),
  };
}

export function useDevicePairingMutations({ accessToken, refreshPairings }) {
  const startOptions = useMemo(
    () => startPairingMutationOptions({ accessToken, refreshPairings }),
    [accessToken, refreshPairings],
  );
  const stopOptions = useMemo(
    () => stopPairingMutationOptions({ accessToken, refreshPairings }),
    [accessToken, refreshPairings],
  );

  const startPairingMutation = useMutation(startOptions);
  const stopPairingMutation = useMutation(stopOptions);

  return {
    startPairingMutation,
    stopPairingMutation,
  };
}
