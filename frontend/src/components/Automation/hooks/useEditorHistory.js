import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

export default function useEditorHistory({
  initialEditor,
  snapshotForSave,
  maxPast = 200,
  batchDelayMs = 800,
}) {
  const [editor, setEditor] = useState(() => (typeof initialEditor === 'function' ? initialEditor() : initialEditor));

  const editorRef = useRef(editor);
  useEffect(() => {
    editorRef.current = editor;
  }, [editor]);

  const historyRef = useRef({ past: [], future: [] });
  const historyBatchRef = useRef(new Map());
  const [historyVersion, setHistoryVersion] = useState(0);

  const bumpHistoryVersion = useCallback(() => setHistoryVersion((v) => v + 1), []);

  const resetHistory = useCallback(() => {
    historyRef.current = { past: [], future: [] };
    historyBatchRef.current = new Map();
    bumpHistoryVersion();
  }, [bumpHistoryVersion]);

  const editorChanged = useCallback((prev, next) => {
    try {
      return snapshotForSave(prev) !== snapshotForSave(next);
    } catch {
      return true;
    }
  }, [snapshotForSave]);

  const pushHistorySnapshot = useCallback((beforeEditor) => {
    historyRef.current.past.push(structuredClone(beforeEditor));
    if (historyRef.current.past.length > maxPast) historyRef.current.past.shift();
    historyRef.current.future = [];
    bumpHistoryVersion();
  }, [bumpHistoryVersion, maxPast]);

  const applyEditorUpdate = useCallback((updater) => {
    setEditor((prev) => {
      const next = updater(prev);
      if (next === prev) return prev;
      if (editorChanged(prev, next)) {
        pushHistorySnapshot(prev);
      }
      return next;
    });
  }, [editorChanged, pushHistorySnapshot]);

  const applyEditorUpdateBatched = useCallback((batchKey, updater) => {
    setEditor((prev) => {
      const next = updater(prev);
      if (next === prev) return prev;
      if (!editorChanged(prev, next)) return next;

      const key = String(batchKey || 'default');
      const batch = historyBatchRef.current;
      const existingTimeout = batch.get(key);
      const hasOpenBatch = !!existingTimeout;
      if (!hasOpenBatch) {
        pushHistorySnapshot(prev);
      }
      if (existingTimeout) window.clearTimeout(existingTimeout);
      const t = window.setTimeout(() => {
        historyBatchRef.current.delete(key);
        bumpHistoryVersion();
      }, batchDelayMs);
      batch.set(key, t);
      bumpHistoryVersion();
      return next;
    });
  }, [batchDelayMs, bumpHistoryVersion, editorChanged, pushHistorySnapshot]);

  const canUndo = useMemo(
    () => historyVersion >= 0 && historyRef.current.past.length > 0,
    [historyVersion],
  );
  const canRedo = useMemo(
    () => historyVersion >= 0 && historyRef.current.future.length > 0,
    [historyVersion],
  );

  const undo = useCallback(() => {
    const past = historyRef.current.past;
    if (!past.length) return;
    const prev = past.pop();
    historyRef.current.future.push(structuredClone(editorRef.current));
    setEditor(prev);
    bumpHistoryVersion();
  }, [bumpHistoryVersion]);

  const redo = useCallback(() => {
    const future = historyRef.current.future;
    if (!future.length) return;
    const next = future.pop();
    historyRef.current.past.push(structuredClone(editorRef.current));
    setEditor(next);
    bumpHistoryVersion();
  }, [bumpHistoryVersion]);

  const commitExternalSnapshot = useCallback((beforeEditor) => {
    const after = editorRef.current;
    if (beforeEditor && after && editorChanged(beforeEditor, after)) {
      pushHistorySnapshot(beforeEditor);
    }
  }, [editorChanged, pushHistorySnapshot]);

  return {
    editor,
    setEditor,
    editorRef,
    historyVersion,
    resetHistory,
    applyEditorUpdate,
    applyEditorUpdateBatched,
    canUndo,
    canRedo,
    undo,
    redo,
    commitExternalSnapshot,
  };
}
