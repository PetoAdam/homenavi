import { useEffect, useState } from 'react';

/**
 * Workflow mutations (create/update/autosave, enable/disable, delete) + editor reset.
 *
 * This hook is intentionally UI-agnostic: it takes callback setters for toast/error and
 * list mutations so Automation.jsx stays mostly orchestration.
 */
export default function useAutomationWorkflowActions({
  accessToken,
  selectedWorkflow,
  editor,
  isDirty,

  buildDefinitionFromEditor,
  editorSnapshotForSave,
  defaultEditorState,

  createWorkflow,
  updateWorkflow,
  enableWorkflow,
  disableWorkflow,
  deleteWorkflow,

  setSelectedId,
  upsertWorkflowInList,
  removeWorkflowFromList,
  fetchWorkflows,

  setEditor,
  setSelectedNodeId,
  resetHistory,

  clearLiveRunHighlights,
  closeRunWs,

  setLastSavedSnapshot,
  setToast,
  setErr,
}) {
  const [saving, setSaving] = useState(false);
  const [lastSavedAt, setLastSavedAt] = useState(null);

  const startNewWorkflow = () => {
    setSelectedId(null);
    setEditor(defaultEditorState());
    setSelectedNodeId('workflow');
    setLastSavedSnapshot('');
    clearLiveRunHighlights();
    closeRunWs();
    resetHistory();
  };

  const saveWorkflowInternal = async ({ silent } = { silent: false }) => {
    if (!accessToken) return null;
    if (saving) return null;

    if (!silent) {
      setErr('');
      setToast('');
    }

    const trimmedName = (editor?.workflowName || '').trim();
    if (!trimmedName) {
      if (!silent) setErr('Workflow name is required');
      return null;
    }

    try {
      const def = buildDefinitionFromEditor(editor);
      setSaving(true);
      const payload = { name: trimmedName, definition: def };

      const res = selectedWorkflow
        ? await updateWorkflow(selectedWorkflow.id, payload, accessToken)
        : await createWorkflow(payload, accessToken);

      setSaving(false);

      if (res.success) {
        let wf = res.data;

        // New workflows should be enabled by default.
        if (!selectedWorkflow && wf?.id && !wf?.enabled) {
          const en = await enableWorkflow(wf.id, accessToken);
          if (en?.success) wf = { ...wf, enabled: true };
        }

        upsertWorkflowInList?.(wf);
        if (!selectedWorkflow && wf?.id) {
          setSelectedId(wf.id);
        }

        if (!silent) setToast(selectedWorkflow ? 'Workflow updated' : 'Workflow created');

        try {
          setLastSavedSnapshot(editorSnapshotForSave(editor));
        } catch {
          // ignore
        }

        setLastSavedAt(Date.now());
        return wf;
      }

      if (!silent) setErr(res.error || 'Save failed');
      return null;
    } catch (ex) {
      setSaving(false);
      if (!silent) setErr(ex?.message || 'Invalid workflow');
      return null;
    }
  };

  // Autosave
  useEffect(() => {
    if (!accessToken) return;
    if (!isDirty) return;

    // Only auto-save if it serializes cleanly.
    try {
      buildDefinitionFromEditor(editor);
    } catch {
      return;
    }

    const t = window.setTimeout(() => {
      saveWorkflowInternal({ silent: true });
    }, 250);
    return () => window.clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accessToken, editor, isDirty]);

  const toggleEnabled = async () => {
    if (!accessToken || !selectedWorkflow) return;
    if (saving) return;

    if (isDirty) {
      const saved = await saveWorkflowInternal({ silent: true });
      if (!saved) {
        setErr('Unable to save changes');
        return;
      }
    }

    setErr('');
    const res = selectedWorkflow.enabled
      ? await disableWorkflow(selectedWorkflow.id, accessToken)
      : await enableWorkflow(selectedWorkflow.id, accessToken);

    if (res.success) {
      setToast(selectedWorkflow.enabled ? 'Disabled' : 'Enabled');
      upsertWorkflowInList?.({ ...selectedWorkflow, enabled: !selectedWorkflow.enabled });
    } else {
      setErr(res.error || 'Failed to update workflow');
    }
  };

  const removeWorkflow = async () => {
    if (!accessToken || !selectedWorkflow) return;
    setErr('');

    const res = await deleteWorkflow(selectedWorkflow.id, accessToken);
    if (res.success) {
      setToast('Workflow deleted');
      const deletedId = selectedWorkflow.id;

      // Optimistically remove from list immediately (avoids stale UI if refresh fails).
      removeWorkflowFromList?.(deletedId);
      startNewWorkflow();
      fetchWorkflows?.();
    } else {
      setErr(res.error || 'Failed to delete workflow');
    }
  };

  return {
    saving,
    lastSavedAt,
    startNewWorkflow,
    saveWorkflowInternal,
    toggleEnabled,
    removeWorkflow,
  };
}
