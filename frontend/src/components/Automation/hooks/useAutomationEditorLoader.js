import { useEffect, useRef } from 'react';

export default function useAutomationEditorLoader({
  selectedId,
  workflows,
  parseWorkflowIntoEditor,
  defaultEditorState,
  setEditor,
  setSelectedNodeId,
  resetHistory,
  setLastSavedSnapshot,
  editorSnapshotForSave,
  onLoadedWorkflowId,
}) {
  const loadedWorkflowIdRef = useRef(null);

  useEffect(() => {
    // Load editor only when selection changes.
    // Avoid re-parsing on autosave upserts (same selectedId) which would reset edit state.
    if (!selectedId) {
      loadedWorkflowIdRef.current = null;
      setEditor(defaultEditorState());
      setSelectedNodeId('workflow');
      setLastSavedSnapshot('');
      resetHistory();
      if (typeof onLoadedWorkflowId === 'function') onLoadedWorkflowId(null);
      return;
    }

    if (loadedWorkflowIdRef.current === selectedId) return;

    const wf = (Array.isArray(workflows) ? workflows : []).find(w => w?.id === selectedId) || null;
    if (!wf) return;

    loadedWorkflowIdRef.current = selectedId;
    const parsed = parseWorkflowIntoEditor(wf);
    setEditor(parsed);
    setSelectedNodeId('workflow');
    resetHistory();
    if (typeof onLoadedWorkflowId === 'function') onLoadedWorkflowId(selectedId);
    try {
      setLastSavedSnapshot(editorSnapshotForSave(parsed));
    } catch {
      setLastSavedSnapshot('');
    }
  }, [
    selectedId,
    workflows,
    parseWorkflowIntoEditor,
    defaultEditorState,
    setEditor,
    setSelectedNodeId,
    resetHistory,
    setLastSavedSnapshot,
    editorSnapshotForSave,
    onLoadedWorkflowId,
  ]);
}
