import { useEffect } from 'react';

export default function useAutomationHotkeys({
  selectedNodeId,
  undo,
  redo,
  deleteSelectedNode,
  enabled = true,
}) {
  useEffect(() => {
    const isTypingTarget = (el) => {
      if (!el) return false;
      const tag = String(el.tagName || '').toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
      return !!el.isContentEditable;
    };

    const onKey = (e) => {
      if (!enabled) return;
      if (isTypingTarget(document.activeElement)) return;

      const key = String(e.key || '');
      const ctrlOrMeta = e.ctrlKey || e.metaKey;

      if (ctrlOrMeta && key.toLowerCase() === 'z' && !e.shiftKey) {
        e.preventDefault();
        undo();
        return;
      }

      if (ctrlOrMeta && ((key.toLowerCase() === 'z' && e.shiftKey) || key.toLowerCase() === 'y')) {
        e.preventDefault();
        redo();
        return;
      }

      if ((key === 'Backspace' || key === 'Delete') && selectedNodeId && selectedNodeId !== 'workflow') {
        e.preventDefault();
        deleteSelectedNode();
      }
    };

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [deleteSelectedNode, enabled, redo, selectedNodeId, undo]);
}
