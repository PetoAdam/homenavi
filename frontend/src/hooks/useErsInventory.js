import { useCallback, useEffect, useMemo, useState } from 'react';

import { listErsDevices, listErsRooms, listErsTags } from '../services/entityRegistryService';
import { getSharedWebSocket, wsUrlForPath } from '../services/realtime/sharedWebSocket';

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

export default function useErsInventory({ enabled, accessToken, realtimeDevices }) {
  const [ersDevices, setErsDevices] = useState([]);
  const [rooms, setRooms] = useState([]);
  const [tags, setTags] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const refresh = useCallback(async (opts = {}) => {
    if (!enabled) return;

    const showLoading = opts?.showLoading !== false;
    if (showLoading) setLoading(true);
    setError('');

    const [devRes, roomRes, tagRes] = await Promise.all([
      listErsDevices(accessToken),
      listErsRooms(accessToken),
      listErsTags(accessToken),
    ]);

    if (!devRes.success) {
      setError(devRes.error || 'Failed to load ERS devices');
      setErsDevices([]);
      setRooms([]);
      setTags([]);
      if (showLoading) setLoading(false);
      return;
    }

    setErsDevices(normalizeArray(devRes.data));
    setRooms(roomRes.success ? normalizeArray(roomRes.data) : []);
    setTags(tagRes.success ? normalizeArray(tagRes.data) : []);
    if (showLoading) setLoading(false);
  }, [accessToken, enabled]);

  useEffect(() => {
    if (!enabled) {
      setErsDevices([]);
      setRooms([]);
      setTags([]);
      setLoading(false);
      setError('');
      return;
    }
    refresh();
  }, [enabled, refresh]);

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
    return items.map((d) => {
      const ersId = safeString(d?.id);
      const hdpIds = normalizeArray(d?.hdp_external_ids).map(safeString).filter(Boolean);
      // Prefer a binding that actually has realtime data, so multi-bound devices still show state.
      const hdpId = hdpIds.find((id) => realtimeByHdpId.has(id)) || hdpIds[0] || '';
      const rt = hdpId ? realtimeByHdpId.get(hdpId) : null;
      const protocol = parseProtocolFromHdpId(hdpId);
      const roomId = d?.room_id ? safeString(d.room_id) : '';
      const room = roomId ? roomById.get(roomId) : null;

      const name = safeString(d?.name) || hdpId || ersId;

      return {
        ...rt,
        ...d,
        ersId,
        hdpIds,
        hdpId: safeString(hdpId),
        id: safeString(hdpId) || ersId,
        protocol: protocol || safeString(rt?.protocol),
        displayName: name,
        name,
        room,
        roomName: safeString(room?.name),
        tags: normalizeArray(d?.tags),
      };
    });
  }, [ersDevices, realtimeByHdpId, roomById]);

  return {
    devices,
    ersDevices,
    rooms,
    tags,
    loading,
    error,
    refresh,
  };
}
