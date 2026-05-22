import { useCallback, useEffect, useMemo, useState } from 'react';

import { listErsDevices, listErsGroups, listErsRooms, listErsTags } from '../services/entityRegistryService';
import { getSharedWebSocket, wsUrlForPath } from '../services/realtime/sharedWebSocket';
import { clearStaleResourceCache, readStaleResourceCache, writeStaleResourceCache } from '../utils/staleResourceCache';

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
  const [ersDevices, setErsDevices] = useState([]);
  const [ersGroups, setErsGroups] = useState([]);
  const [rooms, setRooms] = useState([]);
  const [tags, setTags] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const cacheKey = useMemo(() => {
    if (!accessToken) return '';
    return `homenavi:ers:inventory:${accessToken.slice(-16)}`;
  }, [accessToken]);

  const refresh = useCallback(async (opts = {}) => {
    if (!enabled) return;

    const showLoading = opts?.showLoading !== false;
    if (showLoading) setLoading(true);
    setError('');

    const [devRes, groupRes, roomRes, tagRes] = await Promise.all([
      listErsDevices(accessToken),
      listErsGroups(accessToken),
      listErsRooms(accessToken),
      listErsTags(accessToken),
    ]);

    if (!devRes.success) {
      setError(devRes.error || 'Failed to load ERS devices');
      setErsDevices([]);
      setErsGroups([]);
      setRooms([]);
      setTags([]);
      if (showLoading) setLoading(false);
      return;
    }

    setErsDevices(normalizeArray(devRes.data));
    setErsGroups(groupRes.success ? normalizeArray(groupRes.data) : []);
    setRooms(roomRes.success ? normalizeArray(roomRes.data) : []);
    setTags(tagRes.success ? normalizeArray(tagRes.data) : []);
    writeStaleResourceCache(cacheKey, {
      devices: normalizeArray(devRes.data),
      groups: groupRes.success ? normalizeArray(groupRes.data) : [],
      rooms: roomRes.success ? normalizeArray(roomRes.data) : [],
      tags: tagRes.success ? normalizeArray(tagRes.data) : [],
    });
    if (showLoading) setLoading(false);
  }, [accessToken, cacheKey, enabled]);

  useEffect(() => {
    if (!enabled) {
      setErsDevices([]);
      setErsGroups([]);
      setRooms([]);
      setTags([]);
      setLoading(false);
      setError('');
      clearStaleResourceCache(cacheKey);
      return;
    }
    const cached = readStaleResourceCache(cacheKey, ERS_INVENTORY_CACHE_TTL_MS);
    if (cached) {
      setErsDevices(normalizeArray(cached.devices));
      setErsGroups(normalizeArray(cached.groups));
      setRooms(normalizeArray(cached.rooms));
      setTags(normalizeArray(cached.tags));
      setLoading(false);
      setError('');
      refresh({ showLoading: false });
      return;
    }
    refresh();
  }, [cacheKey, enabled, refresh]);

  useEffect(() => {
    if (!enabled) return undefined;

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
        refresh({ showLoading: false });
      }, 15000);
    };

    const wsUrl = wsUrlForPath('/ws/ers');
    const channel = getSharedWebSocket(wsUrl);

    const unsubMessage = channel.subscribe(() => {
      if (cancelled) return;
      clearRefreshTimer();
      refreshTimer = window.setTimeout(() => {
        refresh({ showLoading: false });
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
  }, [enabled, refresh]);

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
    loading,
    error,
    refresh,
  };
}
