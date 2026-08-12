import { useCallback, useEffect, useMemo, useRef } from 'react';
import { listStatePoints } from '../../../services/historyService';

export function buildHistoryRequest({ fromLocal, toLocal, limitEnabled, limit, order, toRFC3339 }) {
  return {
    from: toRFC3339(fromLocal),
    to: toRFC3339(toLocal),
    limit: limitEnabled ? limit : undefined,
    order,
  };
}

export function normalizeHistoryPoints(points) {
  const source = Array.isArray(points) ? points : [];
  return source.map((point) => {
    let payload = point?.payload;
    if (typeof payload === 'string') {
      try {
        payload = JSON.parse(payload);
      } catch {
        payload = {};
      }
    }
    const normalizedPayload = (payload && typeof payload === 'object' && !Array.isArray(payload))
      ? payload
      : {};
    return {
      ts: point?.ts,
      payload: normalizedPayload,
      retained: Boolean(point?.retained),
      topic: point?.topic || '',
    };
  });
}

export function isLatestHistoryRequest(requestId, latestRequestId) {
  return requestId === latestRequestId;
}

export function useDeviceDetailHistoryQuery({
  deviceId,
  accessToken,
  isResidentOrAdmin,
  fromLocal,
  toLocal,
  limitEnabled,
  limit,
  order,
  toRFC3339,
  setHistoryLoading,
  setHistoryError,
  setHistoryPoints,
}) {
  const autoFetchedRef = useRef(false);
  const latestRequestIdRef = useRef(0);

  const canQueryHistory = useMemo(
    () => Boolean(isResidentOrAdmin && accessToken && deviceId),
    [isResidentOrAdmin, accessToken, deviceId]
  );

  const fetchHistory = useCallback(async () => {
    if (!deviceId) return;
    if (!accessToken) {
      setHistoryError('Authentication required');
      return;
    }

    const requestId = latestRequestIdRef.current + 1;
    latestRequestIdRef.current = requestId;

    setHistoryLoading(true);
    setHistoryError(null);
    try {
      const request = buildHistoryRequest({
        fromLocal,
        toLocal,
        limitEnabled,
        limit,
        order,
        toRFC3339,
      });
      const res = await listStatePoints(deviceId, request, accessToken);
      if (!res.success) {
        throw new Error(res.error || 'Unable to load history');
      }

      if (!isLatestHistoryRequest(requestId, latestRequestIdRef.current)) {
        return;
      }

      const points = normalizeHistoryPoints(res.data?.points);
      setHistoryPoints(points);
    } catch (err) {
      if (!isLatestHistoryRequest(requestId, latestRequestIdRef.current)) {
        return;
      }
      setHistoryError(err?.message || 'Unable to load history');
      setHistoryPoints([]);
    } finally {
      if (isLatestHistoryRequest(requestId, latestRequestIdRef.current)) {
        setHistoryLoading(false);
      }
    }
  }, [
    accessToken,
    deviceId,
    fromLocal,
    limit,
    limitEnabled,
    order,
    setHistoryError,
    setHistoryLoading,
    setHistoryPoints,
    toLocal,
    toRFC3339,
  ]);

  useEffect(() => {
    // Switching devices should re-run the default history query.
    autoFetchedRef.current = false;
    latestRequestIdRef.current += 1;
    setHistoryPoints([]);
    setHistoryError(null);
  }, [deviceId, setHistoryPoints, setHistoryError]);

  useEffect(() => {
    if (!canQueryHistory) return;
    if (autoFetchedRef.current) return;
    if (!fromLocal || !toLocal) return;
    // Auto-run once so the page doesn't look empty with the default range.
    autoFetchedRef.current = true;
    fetchHistory();
  }, [canQueryHistory, fetchHistory, fromLocal, toLocal]);

  return {
    canQueryHistory,
    fetchHistory,
  };
}
