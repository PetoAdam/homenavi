import { useCallback, useEffect, useRef } from 'react';

import { clamp } from '../../mapGeometry';

export default function useMapViewportHandlers({
  svgRef,
  canvasRef,
  mode,
  view,
  setView,
  viewRef,
  panRef,
  minZoom,
  maxZoom,
  handleInsertCornerDragMove,
  handleRoomVertexDragMove,
  handleRoomDragMove,
  handleDeviceDragMove,
  endDeviceDrag,
  endRoomDrag,
  endRoomVertexDrag,
  endInsertCornerDrag,
  handleCanvasPointerMove,
}) {
  const pointersRef = useRef(new Map());
  const pinchRef = useRef(null);
  const handleWheel = useCallback((e) => {
    if (!canvasRef.current) return;
    const delta = e.deltaY;
    if (!Number.isFinite(delta)) return;

    // If the wheel is over the popover and it can scroll, allow scrolling inside the popover.
    const target = e.target;
    const scrollEl = target && typeof target === 'object' && 'closest' in target
      ? target.closest('.map-popover-scroll')
      : null;
    if (scrollEl && typeof scrollEl === 'object') {
      const el = scrollEl;
      const canScrollUp = delta < 0 && el.scrollTop > 0;
      const canScrollDown = delta > 0 && (el.scrollTop + el.clientHeight) < el.scrollHeight;
      if (canScrollUp || canScrollDown) {
        return;
      }
    }

    if (e.cancelable) e.preventDefault();
    e.stopPropagation();

    const v = viewRef.current;

    const zoomFactor = Math.exp(-delta * 0.0012);
    const nextScale = clamp((v.scale || 1) * zoomFactor, minZoom, maxZoom);
    const svg = svgRef.current;
    if (!svg) {
      setView(prev => ({ ...prev, scale: nextScale }));
      return;
    }
    const rect = svg.getBoundingClientRect();
    const cx = (e.clientX ?? 0) - rect.left;
    const cy = (e.clientY ?? 0) - rect.top;
    const worldX = (cx - v.tx) / (v.scale || 1);
    const worldY = (cy - v.ty) / (v.scale || 1);
    const nextTx = cx - worldX * nextScale;
    const nextTy = cy - worldY * nextScale;
    setView({ scale: nextScale, tx: nextTx, ty: nextTy });
  }, [canvasRef, maxZoom, minZoom, setView, svgRef, viewRef]);

  // Capture wheel events at window-level so zoom works even when the cursor is over the popover,
  // and so the page doesn't scroll instead of zooming.
  useEffect(() => {
    const onWheel = (e) => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const target = e.target;
      if (!(target instanceof Node)) return;
      if (!canvas.contains(target)) return;
      handleWheel(e);
    };
    window.addEventListener('wheel', onWheel, { passive: false, capture: true });
    return () => {
      window.removeEventListener('wheel', onWheel, true);
    };
  }, [canvasRef, handleWheel]);

  const handlePointerDown = useCallback((e) => {
    if (e.pointerType === 'touch') {
      pointersRef.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (pointersRef.current.size === 2) {
        const points = Array.from(pointersRef.current.values());
        const [p1, p2] = points;
        const dx = p2.x - p1.x;
        const dy = p2.y - p1.y;
        pinchRef.current = {
          distance: Math.hypot(dx, dy) || 1,
          startView: { scale: view.scale, tx: view.tx, ty: view.ty },
        };
        panRef.current.active = false;
        return;
      }
    }
    if (mode === 'draw') return;
    // allow panning in select mode
    if (e.button !== 0) return;
    panRef.current = {
      active: true,
      startX: e.clientX,
      startY: e.clientY,
      startTx: view.tx,
      startTy: view.ty,
      moved: false,
    };
  }, [mode, panRef, view.tx, view.ty]);

  const handlePointerMove = useCallback((e) => {
    if (e.pointerType === 'touch' && pointersRef.current.has(e.pointerId)) {
      pointersRef.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (pointersRef.current.size >= 2 && pinchRef.current) {
        if (e.cancelable) e.preventDefault();
        const points = Array.from(pointersRef.current.values());
        const [p1, p2] = points;
        if (p1 && p2) {
          const canvasEl = canvasRef.current;
          if (!canvasEl) return;
          const rect = canvasEl.getBoundingClientRect();
          const dx = p2.x - p1.x;
          const dy = p2.y - p1.y;
          const distance = Math.hypot(dx, dy) || 1;
          const zoomFactor = distance / pinchRef.current.distance;
          const nextScale = clamp((pinchRef.current.startView.scale || 1) * zoomFactor, minZoom, maxZoom);
          const midX = (p1.x + p2.x) / 2 - rect.left;
          const midY = (p1.y + p2.y) / 2 - rect.top;
          const worldX = (midX - pinchRef.current.startView.tx) / (pinchRef.current.startView.scale || 1);
          const worldY = (midY - pinchRef.current.startView.ty) / (pinchRef.current.startView.scale || 1);
          const nextTx = midX - worldX * nextScale;
          const nextTy = midY - worldY * nextScale;
          setView({ scale: nextScale, tx: nextTx, ty: nextTy });
          return;
        }
      }
    }
    if (handleInsertCornerDragMove(e)) return;
    if (handleRoomVertexDragMove(e)) return;
    if (handleRoomDragMove(e)) return;
    handleDeviceDragMove(e);
    if (!panRef.current.active) return;
    const dx = (e.clientX ?? 0) - panRef.current.startX;
    const dy = (e.clientY ?? 0) - panRef.current.startY;
    if (Math.abs(dx) + Math.abs(dy) > 3) panRef.current.moved = true;
    setView(prev => ({ ...prev, tx: panRef.current.startTx + dx, ty: panRef.current.startTy + dy }));
  }, [handleDeviceDragMove, handleInsertCornerDragMove, handleRoomDragMove, handleRoomVertexDragMove, panRef, setView]);

  const handleCanvasPointerMoveCombined = useCallback((e) => {
    handlePointerMove(e);
    handleCanvasPointerMove(e);
  }, [handleCanvasPointerMove, handlePointerMove]);

  const handlePointerUp = useCallback((e) => {
    if (e && e.pointerType === 'touch') {
      pointersRef.current.delete(e.pointerId);
      if (pointersRef.current.size < 2) pinchRef.current = null;
    }
    panRef.current.active = false;
    void endDeviceDrag();
    void endRoomDrag();
    void endRoomVertexDrag();
    endInsertCornerDrag();
  }, [endDeviceDrag, endInsertCornerDrag, endRoomDrag, endRoomVertexDrag, panRef]);

  return {
    handleWheel,
    handlePointerDown,
    handlePointerMove,
    handleCanvasPointerMoveCombined,
    handlePointerUp,
  };
}
