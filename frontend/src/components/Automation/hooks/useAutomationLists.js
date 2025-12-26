import { useEffect, useMemo, useState } from 'react';

import { listRuns, listWorkflows } from '../../../services/automationService';
import { listDevices } from '../../../services/deviceHubService';

export default function useAutomationLists({ accessToken, onError } = {}) {
  const [loading, setLoading] = useState(false);

  const [workflows, setWorkflows] = useState([]);
  const [selectedId, setSelectedId] = useState(null);

  const selectedWorkflow = useMemo(
    () => (Array.isArray(workflows) ? workflows.find(w => w?.id === selectedId) : null) || null,
    [workflows, selectedId],
  );

  const [runs, setRuns] = useState([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsLimit, setRunsLimit] = useState(5);

  const [devices, setDevices] = useState([]);
  const [devicesLoading, setDevicesLoading] = useState(false);

  const fetchWorkflows = async () => {
    if (!accessToken) return;
    setLoading(true);

    const wfRes = await listWorkflows(accessToken);
    if (wfRes.success) {
      const next = wfRes.data?.workflows || [];
      setWorkflows(next);

      const currentId = selectedId;
      const hasCurrent = currentId && Array.isArray(next) && next.some(w => w?.id === currentId);
      if ((!currentId || !hasCurrent) && Array.isArray(next) && next.length > 0) {
        const newest = [...next]
          .filter(w => w?.id)
          .sort((a, b) => {
            const at = a?.updated_at ? new Date(a.updated_at).getTime() : 0;
            const bt = b?.updated_at ? new Date(b.updated_at).getTime() : 0;
            return bt - at;
          })[0];
        if (newest?.id) setSelectedId(newest.id);
      }
    } else {
      onError?.(wfRes.error || 'Failed to load workflows');
    }

    setLoading(false);
  };

  const upsertWorkflowInList = (wf) => {
    if (!wf || !wf.id) return;
    setWorkflows(prev => {
      const items = Array.isArray(prev) ? prev : [];
      const idx = items.findIndex(x => x?.id === wf.id);
      if (idx >= 0) {
        const copy = [...items];
        copy[idx] = { ...copy[idx], ...wf };
        return copy;
      }
      return [wf, ...items];
    });
  };

  const removeWorkflowFromList = (workflowId) => {
    const id = workflowId;
    if (!id) return;
    setWorkflows(prev => (Array.isArray(prev) ? prev.filter(w => w?.id !== id) : []));
  };

  const fetchDevices = async () => {
    if (!accessToken) return;
    setDevicesLoading(true);
    const res = await listDevices(accessToken);
    setDevicesLoading(false);
    if (res.success) {
      setDevices(Array.isArray(res.data) ? res.data : (res.data?.devices || []));
    } else {
      setDevices([]);
    }
  };

  const refreshAllData = async () => {
    await Promise.all([fetchWorkflows(), fetchDevices()]);
  };

  const fetchRuns = async (workflowId, runLimit = runsLimit) => {
    if (!accessToken || !workflowId) return [];
    setRunsLoading(true);
    const res = await listRuns(workflowId, accessToken, runLimit);
    setRunsLoading(false);
    if (res.success) {
      const next = res.data?.runs || [];
      setRuns(next);
      return next;
    }
    setRuns([]);
    return [];
  };

  useEffect(() => {
    fetchWorkflows();
    fetchDevices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accessToken]);

  useEffect(() => {
    if (!selectedId) {
      setRuns([]);
      setRunsLimit(5);
      return;
    }
    setRunsLimit(5);
    fetchRuns(selectedId, 5);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId]);

  return {
    loading,

    workflows,
    selectedId,
    setSelectedId,
    selectedWorkflow,
    fetchWorkflows,
    upsertWorkflowInList,
    removeWorkflowFromList,

    runs,
    runsLoading,
    runsLimit,
    setRunsLimit,
    fetchRuns,

    devices,
    devicesLoading,
    fetchDevices,

    refreshAllData,
  };
}
