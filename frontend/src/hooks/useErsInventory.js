import { useCallback, useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';

import { listErsDevices, listErsGroups, listErsRooms, listErsTags } from '../services/entityRegistryService';
import { getSharedWebSocket, wsUrlForPath } from '../services/realtime/sharedWebSocket';
import { clearStaleResourceCache, readStaleResourceCache, writeStaleResourceCache } from '../utils/staleResourceCache';
import { queryKeys } from '../state/queryKeys';

const ERS_INVENTORY_CACHE_TTL_MS = 30 * 1000;

function normalizeArray(value) {
  return Array.isArray(value) ? value : [];
}

function safeString(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function parseProtocolFromHdpId(hdpId) {
  const raw = safeString(hdpId);
  if (!raw) return '';
  const idx = raw.indexOf('/');
  if (idx === -1) return '';
  return raw.slice(0, idx).toLowerCase();
}

export function ersInventoryScopeFromAccessToken(accessToken) {
  return accessToken ? accessToken.slice(-16) : 'anonymous';
}

export async function fetchErsInventorySnapshot(accessToken) {
  const [devRes, groupRes, roomRes, tagRes] = await Promise.all([
    listErsDevices(accessToken),
    listErsGroups(accessToken),
    listErsRooms(accessToken),
    listErsTags(accessToken),
  ]);

  if (!devRes.success) {
    throw new Error(devRes.error || 'Failed to load ERS devices');
  }

  return {
    devices: normalizeArray(devRes.data),
    groups: groupRes.success ? normalizeArray(groupRes.data) : [],
    rooms: roomRes.success ? normalizeArray(roomRes.data) : [],
    tags: tagRes.success ? normalizeArray(tagRes.data) : [],
  };
}

export function ersInventoryQueryOptions(accessToken, { enabled = true, initialData } = {}) {
  const scope = ersInventoryScopeFromAccessToken(accessToken);
  return {
    queryKey: queryKeys.ers.inventory(scope),
    queryFn: async () => fetchErsInventorySnapshot(accessToken),
    enabled,
    staleTime: ERS_INVENTORY_CACHE_TTL_MS,
    initialData,
    initialDataUpdatedAt: typeof initialData !== 'undefined' ? 0 : undefined,
  };
}

function mergeErsDeviceWithRealtime(device, realtimeByHdpId, roomById) {
  const ersId = safeString(device?.id);
  const hdpIds = normalizeArray(device?.hdp_external_ids || device?.hdpIds).map(safeString).filter(Boolean);
  const hdpId = hdpIds.find((id) => realtimeByHdpId.has(id)) || hdpIds[0] || '';
  const rt = hdpId ? realtimeByHdpId.get(hdpId) : null;
  const protocol = parseProtocolFromHdpId(hdpId);
  const roomId = device?.room_id ? safeString(device.room_id) : '';
  const room = roomId ? roomById.get(roomId) : null;
  const name = safeString(device?.name) || hdpId || ersId;

  return {
    ...rt,
    ...device,
    ersId,
    hdpIds,
    hdpId: safeString(hdpId),
    id: safeString(hdpId) || ersId,
    protocol: protocol || safeString(rt?.protocol),
    displayName: name,
    name,
    room,
    roomName: safeString(room?.name),
    tags: normalizeArray(device?.tags),
  };
}

export default function useErsInventory({ enabled, accessToken, realtimeDevices }) {
  const cacheKey = useMemo(() => {
    if (!accessToken) return '';
    return `homenavi:ers:inventory:${ersInventoryScopeFromAccessToken(accessToken)}`;
  }, [accessToken]);
  const queryEnabled = Boolean(enabled && accessToken);
  const cached = useMemo(
    () => (queryEnabled ? readStaleResourceCache(cacheKey, ERS_INVENTORY_CACHE_TTL_MS) : null),
    [cacheKey, queryEnabled],
  );

  const inventoryQuery = useQuery(ersInventoryQueryOptions(accessToken, {
    enabled: queryEnabled,
    initialData: cached
      ? {
        devices: normalizeArray(cached.devices),
        groups: normalizeArray(cached.groups),
        rooms: normalizeArray(cached.rooms),
        tags: normalizeArray(cached.tags),
      }
      : undefined,
  }));

  const ersDevices = normalizeArray(inventoryQuery.data?.devices);
  const ersGroups = normalizeArray(inventoryQuery.data?.groups);
  const rooms = normalizeArray(inventoryQuery.data?.rooms);
  const tags = normalizeArray(inventoryQuery.data?.tags);

  const refresh = useCallback(async () => {
    if (!queryEnabled) return;
    await inventoryQuery.refetch();
  }, [inventoryQuery, queryEnabled]);

  useEffect(() => {
    if (!queryEnabled) {
      clearStaleResourceCache(cacheKey);
      return;
    }
  }, [cacheKey, queryEnabled]);

  useEffect(() => {
    if (!queryEnabled || !inventoryQuery.data) return;
    writeStaleResourceCache(cacheKey, inventoryQuery.data);
  }, [cacheKey, inventoryQuery.data, queryEnabled]);

  useEffect(() => {
    if (!queryEnabled) return undefined;

    let cancelled = false;
    let refreshTimer;
    let pollTimer;

    const clearRefreshTimer = () => {
      if (refreshTimer) window.clearTimeout(refreshTimer);
      refreshTimer = null;
    };

    const clearPollTimer = () => {
      if (pollTimer) window.clearInterval(pollTimer);
      pollTimer = null;
    };

    const ensurePolling = () => {
      if (pollTimer) return;
      pollTimer = window.setInterval(() => {
        if (cancelled) return;
        void refresh();
      }, 15000);
    };

    const wsUrl = wsUrlForPath('/ws/ers');
    const channel = getSharedWebSocket(wsUrl);

    const unsubMessage = channel.subscribe(() => {
      if (cancelled) return;
      clearRefreshTimer();
      refreshTimer = window.setTimeout(() => {
        void refresh();
      }, 150);
    });

    const unsubStatus = channel.onStatus(({ status }) => {
      if (cancelled) return;
      if (status === 'open') {
        clearPollTimer();
      } else if (status === 'closed' || status === 'error') {
        ensurePolling();
      }
    });

    // Safety net until the shared WS opens (or if it's blocked).
    ensurePolling();

    return () => {
      cancelled = true;
      clearRefreshTimer();
      clearPollTimer();

      unsubMessage();
      unsubStatus();
    };
  }, [queryEnabled, refresh]);

  const realtimeByHdpId = useMemo(() => {
    const m = new Map();
    normalizeArray(realtimeDevices).forEach((d) => {
      const id = safeString(d?.hdpId || d?.device_id || d?.id || d?.externalId);
      if (!id) return;
      if (!m.has(id)) m.set(id, d);
    });
    return m;
  }, [realtimeDevices]);

  const roomById = useMemo(() => {
    const m = new Map();
    normalizeArray(rooms).forEach((r) => {
      const id = safeString(r?.id);
      if (!id) return;
      m.set(id, r);
    });
    return m;
  }, [rooms]);

  const devices = useMemo(() => {
    const items = normalizeArray(ersDevices);
    return items.map((d) => mergeErsDeviceWithRealtime(d, realtimeByHdpId, roomById));
  }, [ersDevices, realtimeByHdpId, roomById]);

  const groups = useMemo(() => {
    return normalizeArray(ersGroups).map((group) => {
      const members = normalizeArray(group?.devices).map((device) => mergeErsDeviceWithRealtime(device, realtimeByHdpId, roomById));
      const hdpIds = normalizeArray(group?.hdp_external_ids).map(safeString).filter(Boolean);
      const deviceIds = normalizeArray(group?.device_ids).map(safeString).filter(Boolean);
      return {
        ...group,
        id: safeString(group?.id),
        slug: safeString(group?.slug),
        name: safeString(group?.name) || safeString(group?.slug) || safeString(group?.id),
        description: safeString(group?.description),
        deviceIds,
        hdpIds,
        devices: members,
      };
    });
  }, [ersGroups, realtimeByHdpId, roomById]);

  return {
    devices,
    ersDevices,
    groups,
    ersGroups,
    rooms,
    tags,
    loading: queryEnabled ? inventoryQuery.isLoading : false,
    error: queryEnabled ? (inventoryQuery.error?.message || '') : '',
    refresh,
  };
}
