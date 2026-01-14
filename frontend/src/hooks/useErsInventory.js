import { useCallback, useEffect, useMemo, useState } from 'react';

import { listErsDevices, listErsRooms, listErsTags } from '../services/entityRegistryService';

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
    let ws;
    let reconnectTimer;
    let refreshTimer;
    let pollTimer;
    let reconnectAttempt = 0;

    const clearReconnectTimer = () => {
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    };

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

    const connect = () => {
      if (cancelled) return;
      try {
        const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsUrl = `${proto}://${window.location.host}/ws/ers`;
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
          reconnectAttempt = 0;
          clearPollTimer();
        };

        ws.onmessage = () => {
          if (cancelled) return;
          clearRefreshTimer();
          refreshTimer = window.setTimeout(() => {
            refresh({ showLoading: false });
          }, 150);
        };

        ws.onerror = () => {
          // Let onclose handle backoff + polling.
          try {
            ws?.close();
          } catch {
            // ignore
          }
        };

        ws.onclose = () => {
          if (cancelled) return;
          ensurePolling();
          clearReconnectTimer();
          const delay = Math.min(30000, 1000 * (2 ** reconnectAttempt));
          reconnectAttempt = Math.min(reconnectAttempt + 1, 6);
          reconnectTimer = window.setTimeout(connect, delay);
        };
      } catch {
        ensurePolling();
      }
    };

    connect();

    return () => {
      cancelled = true;
      clearReconnectTimer();
      clearRefreshTimer();
      clearPollTimer();
      try {
        ws?.close();
      } catch {
        // ignore
      }
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
