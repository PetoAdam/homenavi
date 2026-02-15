import { useEffect, useRef, useState } from 'react';

import {
  getDropPosition,
  newNodeId,
  screenToWorld,
  zoomAroundPoint,
} from '../automationUtils';

export default function useAutomationCanvas({
  canvasRef,
  editor,
  editorRef,
  setEditor,
  commitExternalSnapshot,
  applyEditorUpdate,
  setSelectedNodeId,
  defaultNodeData,
  nodeWidth,
  nodeHeaderHeight,
}) {
  const [viewport, setViewport] = useState({ x: 24, y: 24, scale: 1 });
  const [canvasSize, setCanvasSize] = useState({ width: 1, height: 1 });

  const [dragState, setDragState] = useState(null);
  const [panState, setPanState] = useState(null);
  const pointersRef = useRef(new Map());
  const pinchRef = useRef(null);

  const MIN_SCALE = 0.35;
  const MAX_SCALE = 2.5;

  const clampScale = (value) => Math.min(MAX_SCALE, Math.max(MIN_SCALE, value));
  const readPointers = () => Array.from(pointersRef.current.values());

  useEffect(() => {
    const onWheelCapture = (ev) => {
      const canvasEl = canvasRef.current;
      if (!canvasEl) return;
      const t = ev.target;
      if (!(t instanceof Element)) return;
      if (!canvasEl.contains(t)) return;

      // Stop page scrolling while zooming.
      if (ev.cancelable) ev.preventDefault();
      ev.stopPropagation();

      const rect = canvasEl.getBoundingClientRect();
      const point = { x: ev.clientX - rect.left, y: ev.clientY - rect.top };
      const factor = Math.pow(1.0015, -ev.deltaY);
      setViewport((prev) => zoomAroundPoint(prev, point, prev.scale * factor));
    };

    const opts = { passive: false, capture: true };
    window.addEventListener('wheel', onWheelCapture, opts);
    return () => window.removeEventListener('wheel', onWheelCapture, opts);
  }, [canvasRef]);

  const onCanvasPointerDown = (e) => {
    if (e.pointerType !== 'touch') return;
    pointersRef.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointersRef.current.size !== 2) return;

    const canvasEl = canvasRef.current;
    if (!canvasEl) return;
    const rect = canvasEl.getBoundingClientRect();
    const [p1, p2] = readPointers();
    if (!p1 || !p2) return;

    const mid = {
      x: (p1.x + p2.x) / 2 - rect.left,
      y: (p1.y + p2.y) / 2 - rect.top,
    };
    const dx = p2.x - p1.x;
    const dy = p2.y - p1.y;
    const distance = Math.hypot(dx, dy);
    const startView = { x: viewport.x, y: viewport.y, scale: viewport.scale };
    const startWorld = screenToWorld(mid, startView);
    pinchRef.current = {
      distance: distance || 1,
      startView,
      startWorld,
    };
  };

  const onCanvasPointerMove = (e) => {
    if (e.pointerType !== 'touch') return;
    if (!pointersRef.current.has(e.pointerId)) return;
    pointersRef.current.set(e.pointerId, { x: e.clientX, y: e.clientY });

    if (pointersRef.current.size < 2 || !pinchRef.current) return;
    if (e.cancelable) e.preventDefault();

    const canvasEl = canvasRef.current;
    if (!canvasEl) return;
    const rect = canvasEl.getBoundingClientRect();
    const [p1, p2] = readPointers();
    if (!p1 || !p2) return;

    const dx = p2.x - p1.x;
    const dy = p2.y - p1.y;
    const distance = Math.hypot(dx, dy) || 1;
    const scale = clampScale(pinchRef.current.startView.scale * (distance / pinchRef.current.distance));
    const mid = {
      x: (p1.x + p2.x) / 2 - rect.left,
      y: (p1.y + p2.y) / 2 - rect.top,
    };
    const nextX = mid.x - pinchRef.current.startWorld.x * scale;
    const nextY = mid.y - pinchRef.current.startWorld.y * scale;
    setViewport({ x: nextX, y: nextY, scale });
  };

  const onCanvasPointerUp = (e) => {
    if (e.pointerType !== 'touch') return;
    pointersRef.current.delete(e.pointerId);
    if (pointersRef.current.size < 2) pinchRef.current = null;
  };

  useEffect(() => {
    const canvasEl = canvasRef.current;
    if (!canvasEl) return;

    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      const cr = entry.contentRect;
      setCanvasSize({ width: cr.width || 1, height: cr.height || 1 });
    });

    ro.observe(canvasEl);
    return () => ro.disconnect();
  }, [canvasRef]);

  useEffect(() => {
    if (!dragState) return;

    const onMove = (e) => {
      setEditor(prev => {
        const canvasEl = canvasRef.current;
        if (!canvasEl) return prev;

        const rect = canvasEl.getBoundingClientRect();
        const screen = { x: e.clientX - rect.left, y: e.clientY - rect.top };
        const world = screenToWorld(screen, viewport);
        const nodes = Array.isArray(prev.nodes) ? prev.nodes : [];
        const idx = nodes.findIndex(n => n.id === dragState.nodeId);
        if (idx < 0) return prev;
        const node = nodes[idx];
        const x = world.x - dragState.offsetWorldX;
        const y = world.y - dragState.offsetWorldY;
        const nextNodes = [...nodes];
        nextNodes[idx] = { ...node, x, y };
        return { ...prev, nodes: nextNodes };
      });
    };

    const onUp = () => {
      const before = dragState?.historyBefore;
      if (before) commitExternalSnapshot(before);
      setDragState(null);
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);

    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [canvasRef, commitExternalSnapshot, dragState, setEditor, viewport]);

  useEffect(() => {
    if (!panState) return;

    const onMove = (e) => {
      setViewport(prev => ({
        ...prev,
        x: panState.startX + (e.clientX - panState.pointerStartX),
        y: panState.startY + (e.clientY - panState.pointerStartY),
      }));
    };

    const onUp = () => {
      setPanState(null);
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [panState]);

  const onPaletteDragStart = (e, type) => {
    e.dataTransfer.setData('text/plain', type);
    e.dataTransfer.effectAllowed = 'copy';
  };

  const onCanvasDragOver = (e) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  };

  const onCanvasDrop = (e) => {
    e.preventDefault();
    const canvasEl = canvasRef.current;
    if (!canvasEl) return;

    const type = e.dataTransfer.getData('text/plain');
    const pos = getDropPosition(e, canvasEl);
    const world = screenToWorld(pos, viewport);

    const createdId = type.startsWith('trigger.') ? newNodeId('trigger') : null;

    applyEditorUpdate(prev => {
      const x = world.x - nodeWidth / 2;
      const y = world.y - nodeHeaderHeight / 2;

      const nodes = Array.isArray(prev.nodes) ? prev.nodes : [];

      if (type.startsWith('trigger.')) {
        return { ...prev, nodes: [...nodes, { id: createdId, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'action.send_command') {
        const id = newNodeId('action');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'action.notify_email') {
        const id = newNodeId('email');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.sleep') {
        const id = newNodeId('sleep');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.if') {
        const id = newNodeId('if');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.for') {
        const id = newNodeId('for');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      return prev;
    });

    if (type.startsWith('trigger.') && createdId) setSelectedNodeId(createdId);
  };

  const onNodePointerDown = (e, nodeId) => {
    const canvasEl = canvasRef.current;
    if (!canvasEl) return;

    e.stopPropagation();
    const rect = canvasEl.getBoundingClientRect();
    const screen = { x: e.clientX - rect.left, y: e.clientY - rect.top };
    const world = screenToWorld(screen, viewport);
    const nodes = Array.isArray(editor.nodes) ? editor.nodes : [];
    const node = nodes.find(n => n.id === nodeId);
    if (!node) return;

    const dragBefore = structuredClone(editorRef.current);

    setDragState({
      nodeId,
      offsetWorldX: world.x - node.x,
      offsetWorldY: world.y - node.y,
      historyBefore: dragBefore,
    });
  };

  const beginPan = (e) => {
    if (pinchRef.current || pointersRef.current.size > 1) return;
    setSelectedNodeId('workflow');
    setPanState({
      pointerStartX: e.clientX,
      pointerStartY: e.clientY,
      startX: viewport.x,
      startY: viewport.y,
    });
  };

  const addNodeAtCenter = (type) => {
    const canvasEl = canvasRef.current;
    if (!canvasEl) return;
    const centerScreen = { x: canvasSize.width / 2, y: canvasSize.height / 2 };
    const world = screenToWorld(centerScreen, viewport);
    const x = world.x - nodeWidth / 2;
    const y = world.y - nodeHeaderHeight / 2;

    const createdId = type.startsWith('trigger.') ? newNodeId('trigger') : null;

    applyEditorUpdate(prev => {
      const nodes = Array.isArray(prev.nodes) ? prev.nodes : [];

      if (type.startsWith('trigger.')) {
        return { ...prev, nodes: [...nodes, { id: createdId, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'action.send_command') {
        const id = newNodeId('action');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'action.notify_email') {
        const id = newNodeId('email');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.sleep') {
        const id = newNodeId('sleep');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.if') {
        const id = newNodeId('if');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      if (type === 'logic.for') {
        const id = newNodeId('for');
        return { ...prev, nodes: [...nodes, { id, kind: type, x, y, data: defaultNodeData(type) }] };
      }

      return prev;
    });

    if (type.startsWith('trigger.') && createdId) setSelectedNodeId(createdId);
  };

  return {
    viewport,
    setViewport,
    canvasSize,

    onPaletteDragStart,
    onCanvasDragOver,
    onCanvasDrop,
    onNodePointerDown,
    beginPan,
    addNodeAtCenter,
    onCanvasPointerDown,
    onCanvasPointerMove,
    onCanvasPointerUp,
  };
}
