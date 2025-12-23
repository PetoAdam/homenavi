import { useEffect, useRef, useState } from 'react';

import { screenToWorld } from '../automationUtils';

export default function useAutomationConnectMode({
  editorNodes,
  viewport,
  canvasRef,
  applyEditorUpdate,
  isTriggerNode,
  nodeWidth,
  nodeHeaderHeight,
  setSelectedNodeId,
}) {
  const connectModeRef = useRef(null);
  const [connectMode, setConnectMode] = useState(null);
  const [connectHoverId, setConnectHoverId] = useState(null);

  const commitConnection = (fromId, toId) => {
    const from = String(fromId);
    const to = String(toId);
    if (!from || !to || from === to) return;

    applyEditorUpdate(prev => {
      const nodes = Array.isArray(prev.nodes) ? prev.nodes : [];
      const nodeById = new Map(nodes.map(n => [String(n?.id), n]));
      const toNode = nodeById.get(to);
      if (!toNode) return prev;
      if (isTriggerNode(toNode)) return prev;
      const edges = Array.isArray(prev.edges) ? prev.edges : [];
      const exists = edges.some(e => String(e?.from) === from && String(e?.to) === to);
      if (exists) return prev;
      return { ...prev, edges: [...edges, { from, to }] };
    });
  };

  const cancelConnect = () => {
    connectModeRef.current = null;
    setConnectMode(null);
    setConnectHoverId(null);
  };

  const beginConnect = ({ fromId, mode }) => {
    const nodes = Array.isArray(editorNodes) ? editorNodes : [];
    const from = nodes.find(n => n && String(n.id) === String(fromId));
    if (!from) return;

    const fromX = Number(from.x);
    const fromY = Number(from.y);
    if (!Number.isFinite(fromX) || !Number.isFinite(fromY)) return;

    setSelectedNodeId(String(fromId));

    const x1 = fromX + nodeWidth;
    const y1 = fromY + nodeHeaderHeight / 2;

    // Start preview at the output port so a click without drag can fall back to click-mode.
    const x2 = x1;
    const y2 = y1;

    const next = { fromId: String(fromId), mode, x1, y1, x2, y2 };
    connectModeRef.current = next;
    setConnectMode(next);
    setConnectHoverId(null);
  };

  const startConnectFromNode = (e, fromId) => {
    e.stopPropagation();
    e.preventDefault();

    // Touch users: tap-to-connect. Mouse users: drag-to-connect.
    const mode = e.pointerType === 'mouse' ? 'drag' : 'click';
    beginConnect({ fromId, mode });
  };

  useEffect(() => {
    if (!connectMode || connectMode.mode !== 'drag') return;

    const pickTarget = (cs) => {
      const fromId = String(cs.fromId);
      const nodes = Array.isArray(editorNodes) ? editorNodes : [];
      let best = null;
      nodes.forEach(n => {
        if (!n) return;
        if (String(n.id) === fromId) return;
        if (isTriggerNode(n)) return;
        const x = n.x;
        const y = n.y + nodeHeaderHeight / 2;
        const dist = Math.hypot(cs.x2 - x, cs.y2 - y);
        if (dist <= 30 && (!best || dist < best.dist)) best = { id: String(n.id), dist, x, y };
      });
      return best;
    };

    const onMove = (e) => {
      const canvasEl = canvasRef.current;
      if (!canvasEl) return;

      const rect = canvasEl.getBoundingClientRect();
      const screen = { x: e.clientX - rect.left, y: e.clientY - rect.top };
      const world = screenToWorld(screen, viewport);

      setConnectMode(prev => {
        if (!prev) return prev;
        const next = { ...prev, x2: world.x, y2: world.y };
        connectModeRef.current = next;
        const best = pickTarget(next);
        setConnectHoverId(best ? best.id : null);
        return next;
      });
    };

    const onUp = () => {
      const cs = connectModeRef.current;
      if (cs && cs.fromId) {
        const best = pickTarget(cs);
        if (best) {
          commitConnection(cs.fromId, best.id);
          cancelConnect();
          return;
        }

        // If the user clicked (no drag), keep connect mode and let them click a target.
        const moved = Math.hypot(cs.x2 - cs.x1, cs.y2 - cs.y1);
        if (moved < 6) {
          const next = { ...cs, mode: 'click', x2: cs.x1, y2: cs.y1 };
          connectModeRef.current = next;
          setConnectMode(next);
          setConnectHoverId(null);
          return;
        }
      }

      cancelConnect();
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [connectMode, viewport, editorNodes, canvasRef, isTriggerNode, nodeHeaderHeight]);

  useEffect(() => {
    if (!connectMode) return;
    const onKey = (e) => {
      if (e.key === 'Escape') cancelConnect();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [connectMode]);

  return {
    connectModeRef,
    connectMode,
    setConnectMode,
    connectHoverId,
    setConnectHoverId,
    cancelConnect,
    commitConnection,
    startConnectFromNode,
  };
}
