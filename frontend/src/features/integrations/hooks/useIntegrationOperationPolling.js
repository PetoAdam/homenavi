import { useEffect, useMemo } from 'react';
import { getIntegrationInstallStatus } from '../../../services/integrationService';

export function collectActiveOperationIds({ installing = {}, updating = {}, restarting = {} }) {
  const ids = new Set();
  Object.entries(installing).forEach(([id, active]) => {
    if (active) ids.add(id);
  });
  Object.entries(updating).forEach(([id, active]) => {
    if (active) ids.add(id);
  });
  Object.entries(restarting).forEach(([id, active]) => {
    if (active) ids.add(id);
  });
  return Array.from(ids);
}

export function useIntegrationOperationPolling({
  installing,
  updating,
  restarting,
  pollIntervalMs = 1500,
  getStatus = getIntegrationInstallStatus,
  onStatus,
  onInstallTerminal,
  onUpdateTerminal,
  onRestartTerminal,
  isTerminalOperationStatus,
}) {
  const activeIds = useMemo(
    () => collectActiveOperationIds({ installing, updating, restarting }),
    [installing, updating, restarting]
  );

  useEffect(() => {
    if (!activeIds.length) return undefined;
    let cancelled = false;

    const poll = async () => {
      await Promise.all(activeIds.map(async (id) => {
        const res = await getStatus(id);
        if (!res?.success || cancelled) return;

        const status = res.data;
        if (typeof onStatus === 'function') {
          onStatus(id, status);
        }

        if (!isTerminalOperationStatus(status)) return;

        if (installing?.[id] && typeof onInstallTerminal === 'function') {
          onInstallTerminal(id, status);
        }
        if (updating?.[id] && typeof onUpdateTerminal === 'function') {
          onUpdateTerminal(id, status);
        }
        if (restarting?.[id] && typeof onRestartTerminal === 'function') {
          onRestartTerminal(id, status);
        }
      }));
    };

    poll();
    const handle = setInterval(poll, pollIntervalMs);

    return () => {
      cancelled = true;
      clearInterval(handle);
    };
  }, [
    activeIds,
    getStatus,
    onInstallTerminal,
    onRestartTerminal,
    onStatus,
    onUpdateTerminal,
    installing,
    isTerminalOperationStatus,
    pollIntervalMs,
    restarting,
    updating,
  ]);
}
