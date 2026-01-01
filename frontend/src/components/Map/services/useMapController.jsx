import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import useDeviceHubDevices from '../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../hooks/useErsInventory';
import { useAuth } from '../../../context/AuthContext';
import { createErsRoom, deleteErsRoom, patchErsDevice, patchErsRoom } from '../../../services/entityRegistryService';

import usePersistedLayout from './mapController/usePersistedLayout';
import useDevicePalette from './mapController/useDevicePalette';
import useMapViewportHandlers from './mapController/useMapViewportHandlers';

import { formatMetricValueAndUnitForKey } from '../../../utils/stateFormat';

import {
  clamp,
  closestPointOnSegment,
  computeSegmentLengths,
  dist,
  pointInPolygon,
  roomPolygonToPath,
  applyLengthToSegment,
} from '../mapGeometry';
import {
  normalizeNumberOrNull as normalizeNumber,
  normalizePointArrayFromMeta as normalizePointArray,
  readFavoriteFieldsFromErsMeta,
  safeString,
} from '../mapErsMeta';
import {
  collectFavoriteFieldOptionsFromState,
  formatRelativeTimeShort,
  getFaSvgPath,
  iconForFactLabel,
  lowerKey,
  pickStateValue,
  uniqueRoomName,
} from '../mapDeviceUtils';
import {
  collectSnapTargets as collectSnapTargetsImpl,
  snapAxisToVertices as snapAxisToVerticesImpl,
  snapForDraw as snapForDrawImpl,
  snapPoint as snapPointImpl,
  snapToGrid as snapToGridImpl,
} from '../mapSnapping';
import { mergeDevicePlacementsFromErs, mergeRoomsFromErs } from '../mapHydrate';

const STORAGE_KEY = 'homenavi.map.layout.v1';
const SNAP_DISTANCE_PX = 10;
// Snap within ~5 degrees of horizontal/vertical.
const ORTHO_RATIO = Math.tan((5 * Math.PI) / 180);
const GRID_SIZE = 28;
const MIN_ZOOM = 0.35;
const MAX_ZOOM = 3.2;

function computeWorldBoundsFromLayout(rooms, devicePlacements) {
  const pts = [];
  (Array.isArray(rooms) ? rooms : []).forEach((r) => {
    (Array.isArray(r?.points) ? r.points : []).forEach((p) => {
      const x = Number(p?.x);
      const y = Number(p?.y);
      if (Number.isFinite(x) && Number.isFinite(y)) pts.push({ x, y });
    });
  });

  if (devicePlacements && typeof devicePlacements === 'object') {
    Object.values(devicePlacements).forEach((pl) => {
      const x = Number(pl?.x);
      const y = Number(pl?.y);
      if (Number.isFinite(x) && Number.isFinite(y)) pts.push({ x, y });
    });
  }

  if (!pts.length) return null;
  let minX = pts[0].x;
  let minY = pts[0].y;
  let maxX = pts[0].x;
  let maxY = pts[0].y;
  for (let i = 1; i < pts.length; i += 1) {
    const p = pts[i];
    if (p.x < minX) minX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.x > maxX) maxX = p.x;
    if (p.y > maxY) maxY = p.y;
  }
  return { minX, minY, maxX, maxY };
}

export default function useMapController() {
  const navigate = useNavigate();
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const {
    devices: realtimeDevices,
    loading: realtimeLoading,
  } = useDeviceHubDevices({ enabled: Boolean(isResidentOrAdmin), metadataMode: 'ws' });

  const {
    devices,
    rooms: ersRooms,
    loading: ersLoading,
    error: ersError,
    refresh: refreshErsInventory,
  } = useErsInventory({ enabled: Boolean(isResidentOrAdmin), accessToken, realtimeDevices });

  const svgRef = useRef(null);
  const canvasRef = useRef(null);

  const layoutSnapshotForSave = useCallback((ed) => {
    try {
      // Only track room edits in history (devices can be added later if desired).
      return JSON.stringify(Array.isArray(ed?.rooms) ? ed.rooms : []);
    } catch {
      return '';
    }
  }, []);

  const {
    editor: layout,
    setEditor: setLayout,
    editorRef: layoutRef,
    canUndo,
    canRedo,
    undo,
    redo,
    applyEditorUpdate,
    applyEditorUpdateBatched,
  } = usePersistedLayout({ storageKey: STORAGE_KEY, snapshotForSave: layoutSnapshotForSave });

  const [mode, setMode] = useState('select'); // select | draw
  const [editEnabled, setEditEnabled] = useState(false);
  const [activeRoomId, setActiveRoomId] = useState('');
  const [activeVertexIndex, setActiveVertexIndex] = useState(null);
  const [draft, setDraft] = useState(null); // { id, name, points: [], wallLengths: [] }
  const [hoverPoint, setHoverPoint] = useState(null);
  const [selectedDeviceId, setSelectedDeviceId] = useState(''); // mobile click-to-place
  const [activeWallIndex, setActiveWallIndex] = useState(null);
  const [mapError, setMapError] = useState('');
  const [opPending, setOpPending] = useState(false);
  const [roomNameEdit, setRoomNameEdit] = useState('');

  const [expandedDeviceKey, setExpandedDeviceKey] = useState('');
  const [favoritesEditorKey, setFavoritesEditorKey] = useState('');
  const dragDeviceRef = useRef(null); // { key, offsetX, offsetY, lastX, lastY, moved }
  const suppressDeviceClickRef = useRef(false);
  const dragRoomRef = useRef(null); // { roomId, startWorld, startPoints, moved, lastDx, lastDy }
  const dragRoomVertexRef = useRef(null); // { roomId, vertexIndex, moved, startPoints }
  const suppressRoomClickRef = useRef(false);
  const dragInsertCornerRef = useRef(null); // { roomId, segIndex, moved }
  const [insertCornerPreview, setInsertCornerPreview] = useState(null); // { roomId, segIndex, point }

  useEffect(() => {
    if (!expandedDeviceKey) {
      setFavoritesEditorKey('');
      return;
    }
    setFavoritesEditorKey(prev => (prev && prev !== expandedDeviceKey ? '' : prev));
  }, [expandedDeviceKey]);

  const [view, setView] = useState({ scale: 1, tx: 0, ty: 0 });
  const viewRef = useRef({ scale: 1, tx: 0, ty: 0 });
  const panRef = useRef({ active: false, startX: 0, startY: 0, startTx: 0, startTy: 0, moved: false });
  const didAutoCenterRef = useRef(false);

  useEffect(() => {
    viewRef.current = view;
  }, [view]);

  const [snapSettings, setSnapSettings] = useState({
    vertex: true,
    edge: true,
    align: true,
    ortho: true,
    grid: false,
  });
  const [snapGuide, setSnapGuide] = useState(null); // { kind, x1,y1,x2,y2, px, py }

  const rooms = useMemo(() => (Array.isArray(layout.rooms) ? layout.rooms : []), [layout.rooms]);

  const persistAfterHistoryNavRef = useRef(false);
  const preservePlacementsAfterHistoryNavRef = useRef(null);

  const { devicesForPalette, deviceByKey } = useDevicePalette({ devices });

  // Hydrate local layout from ERS room meta when available (e.g. fresh browser).
  useEffect(() => {
    if (!Array.isArray(ersRooms) || ersRooms.length === 0) return;
    setLayout(prev => mergeRoomsFromErs(prev, ersRooms));
  }, [ersRooms, setLayout]);

  // Hydrate device placements from ERS device meta when available.
  useEffect(() => {
    if (!Array.isArray(devicesForPalette) || devicesForPalette.length === 0) return;
    setLayout(prev => mergeDevicePlacementsFromErs(prev, devicesForPalette, dragDeviceRef.current?.key || ''));
  }, [devicesForPalette, setLayout]);

  // Lightweight collaboration support (no websockets): poll ERS inventory while edit mode is open.
  useEffect(() => {
    if (!editEnabled) return;
    if (typeof refreshErsInventory !== 'function') return;
    const id = window.setInterval(() => {
      refreshErsInventory();
    }, 5000);
    return () => window.clearInterval(id);
  }, [editEnabled, refreshErsInventory]);

  const activeRoom = useMemo(() => {
    if (draft && mode === 'draw') return draft;
    if (!activeRoomId) return null;
    return rooms.find(r => r.id === activeRoomId) || null;
  }, [activeRoomId, draft, mode, rooms]);

  useEffect(() => {
    if (mode === 'draw') {
      setRoomNameEdit('');
      setActiveVertexIndex(null);
      return;
    }
    const r = rooms.find(x => x.id === activeRoomId);
    setRoomNameEdit(safeString(r?.name));
    setActiveVertexIndex(null);
  }, [activeRoomId, mode, rooms]);

  const snapWorld = useMemo(() => (SNAP_DISTANCE_PX / (view.scale || 1)), [view.scale]);

  const svgPointFromEvent = useCallback((evt) => {
    const svg = svgRef.current;
    if (!svg) return null;
    const rect = svg.getBoundingClientRect();
    const x = (evt.clientX ?? 0) - rect.left;
    const y = (evt.clientY ?? 0) - rect.top;
    const scale = view.scale || 1;
    const worldX = (x - view.tx) / scale;
    const worldY = (y - view.ty) / scale;
    return { x: worldX, y: worldY };
  }, [view.scale, view.tx, view.ty]);

  const snapTargets = useMemo(() => collectSnapTargetsImpl(rooms, draft), [draft, rooms]);

  const snapAxisToVertices = useCallback((rawPoint, excludePoint) => {
    return snapAxisToVerticesImpl(rawPoint, snapTargets.vertices, snapWorld, excludePoint);
  }, [snapTargets.vertices, snapWorld]);

  const snapPoint = useCallback((p, opts = {}) => {
    return snapPointImpl(p, snapTargets.vertices, snapTargets.segments, snapWorld, opts);
  }, [snapTargets.segments, snapTargets.vertices, snapWorld]);

  const snapToGrid = useCallback((p) => {
    return snapToGridImpl(p, GRID_SIZE, snapWorld);
  }, [snapWorld]);

  const snapForDraw = useCallback((prevPoint, rawPoint, firstPoint) => {
    return snapForDrawImpl(prevPoint, rawPoint, firstPoint, {
      vertices: snapTargets.vertices,
      segments: snapTargets.segments,
      snapSettings,
      snapWorld,
      gridSize: GRID_SIZE,
      orthoRatio: ORTHO_RATIO,
    });
  }, [snapSettings, snapTargets.segments, snapTargets.vertices, snapWorld]);

  const existingNamesLower = useMemo(() => {
    const set = new Set();
    (Array.isArray(ersRooms) ? ersRooms : []).forEach(r => {
      const n = lowerKey(r?.name);
      if (n) set.add(n);
    });
    rooms.forEach(r => {
      const n = lowerKey(r?.name);
      if (n) set.add(n);
    });
    return set;
  }, [ersRooms, rooms]);

  const startRoom = useCallback(() => {
    if (!editEnabled) return;
    const id = `room_${Math.random().toString(16).slice(2)}`;
    setMode('draw');
    const name = uniqueRoomName('Room', existingNamesLower);
    setDraft({ id, name, points: [], wallLengths: [] });
    setActiveRoomId('');
    setActiveWallIndex(null);
    setMapError('');
  }, [editEnabled, existingNamesLower]);

  const cancelDraft = useCallback(() => {
    setDraft(null);
    setMode('select');
    setHoverPoint(null);
    setActiveWallIndex(null);
    setMapError('');
  }, []);

  useEffect(() => {
    if (editEnabled) return;
    cancelDraft();
    setActiveRoomId('');
    setActiveWallIndex(null);
    setSelectedDeviceId('');
  }, [cancelDraft, editEnabled]);

  const finalizeDraft = useCallback(async () => {
    if (!draft) return;
    const pts = Array.isArray(draft.points) ? draft.points : [];
    if (pts.length < 3) return;
    if (!accessToken) {
      setMapError('Authentication required');
      return;
    }

    const desired = safeString(draft.name).trim() || 'Room';
    const name = uniqueRoomName(desired, existingNamesLower);

    setOpPending(true);
    setMapError('');
    try {
      const meta = {
        map: {
          points: normalizePointArray(pts),
          wall_lengths: Array.isArray(draft.wallLengths) ? draft.wallLengths : [],
        },
      };
      const res = await createErsRoom({ name, meta }, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to create room');
      const ersRoomId = safeString(res.data?.id);
      if (!ersRoomId) throw new Error('Room created but missing id');

      const newRoom = {
        id: ersRoomId,
        name,
        points: pts,
        wallLengths: Array.isArray(draft.wallLengths) ? draft.wallLengths : [],
      };
      setLayout(prev => ({
        ...prev,
        rooms: [...(Array.isArray(prev.rooms) ? prev.rooms : []), newRoom],
      }));
      setDraft(null);
      setMode('select');
      setActiveRoomId(ersRoomId);
      setActiveWallIndex(null);
    } catch (err) {
      setMapError(err?.message || 'Unable to create room');
    } finally {
      setOpPending(false);
    }
  }, [accessToken, draft, existingNamesLower, setLayout]);

  const updateRoomName = useCallback(async (roomId, name) => {
    const desired = safeString(name).trim();
    const others = new Set(existingNamesLower);
    // allow keeping its current name by removing it from the taken set
    const current = rooms.find(r => r.id === roomId);
    if (current?.name) {
      others.delete(lowerKey(current.name));
    }
    const unique = desired ? uniqueRoomName(desired, others) : uniqueRoomName('Room', others);
    setLayout(prev => ({
      ...prev,
      rooms: (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => (r.id === roomId ? { ...r, name: unique } : r)),
    }));

    if (accessToken && roomId) {
      await patchErsRoom(roomId, { name: unique }, accessToken);
    }
  }, [accessToken, existingNamesLower, rooms, setLayout]);

  const persistRoomGeometry = useCallback(async (roomId, points, wallLengths) => {
    if (!accessToken || !roomId) return;
    const meta = {
      map: {
        points: normalizePointArray(points),
        wall_lengths: Array.isArray(wallLengths) ? wallLengths : [],
      },
    };
    await patchErsRoom(roomId, { meta }, accessToken);
  }, [accessToken]);

  const handleUndo = useCallback(() => {
    if (!canUndo) return;
    preservePlacementsAfterHistoryNavRef.current = layoutRef.current?.devicePlacements || {};
    persistAfterHistoryNavRef.current = true;
    undo();
  }, [canUndo, layoutRef, undo]);

  const handleRedo = useCallback(() => {
    if (!canRedo) return;
    preservePlacementsAfterHistoryNavRef.current = layoutRef.current?.devicePlacements || {};
    persistAfterHistoryNavRef.current = true;
    redo();
  }, [canRedo, layoutRef, redo]);

  useEffect(() => {
    if (!persistAfterHistoryNavRef.current) return;
    persistAfterHistoryNavRef.current = false;

    // Keep device placements stable across undo/redo (history is room-focused).
    const preservedPlacements = preservePlacementsAfterHistoryNavRef.current;
    preservePlacementsAfterHistoryNavRef.current = null;
    if (preservedPlacements) {
      setLayout(prev => ({
        ...prev,
        devicePlacements: preservedPlacements,
      }));
    }

    const rs = Array.isArray(layoutRef.current?.rooms) ? layoutRef.current.rooms : [];
    void Promise.all(rs.map(r => persistRoomGeometry(r.id, r.points, r.wallLengths)));
  }, [layout, layoutRef, persistRoomGeometry, setLayout]);

  const beginRoomDrag = useCallback((e, roomId) => {
    if (!editEnabled) return;
    if (mode === 'draw') return;
    if (!roomId) return;
    if (e.button !== 0) return;
    const room = rooms.find(r => r.id === roomId);
    const pts = Array.isArray(room?.points) ? room.points : [];
    if (pts.length < 3) return;
    const p = svgPointFromEvent(e);
    if (!p) return;

    e.stopPropagation();
    panRef.current.active = false;

    setActiveRoomId(roomId);
    setActiveWallIndex(null);
    setActiveVertexIndex(null);

    dragRoomRef.current = {
      roomId,
      startWorld: p,
      startPoints: pts.map(pt => ({ x: pt.x, y: pt.y })),
      moved: false,
      lastDx: 0,
      lastDy: 0,
    };
    suppressRoomClickRef.current = false;

    try {
      e.currentTarget?.setPointerCapture?.(e.pointerId);
    } catch {
      // ignore
    }
  }, [editEnabled, mode, rooms, svgPointFromEvent]);

  const handleRoomDragMove = useCallback((e) => {
    const st = dragRoomRef.current;
    if (!st) return false;
    const p = svgPointFromEvent(e);
    if (!p) return true;

    const dx = p.x - st.startWorld.x;
    const dy = p.y - st.startWorld.y;

    if (Math.abs(dx - (st.lastDx || 0)) + Math.abs(dy - (st.lastDy || 0)) > 0.25) st.moved = true;
    st.lastDx = dx;
    st.lastDy = dy;

    applyEditorUpdateBatched(`room-drag:${st.roomId}`, prev => {
      const nextRooms = (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => {
        if (r.id !== st.roomId) return r;
        const movedPts = st.startPoints.map(pt => ({ x: pt.x + dx, y: pt.y + dy }));
        return { ...r, points: movedPts };
      });
      return { ...prev, rooms: nextRooms };
    });
    return true;
  }, [applyEditorUpdateBatched, svgPointFromEvent]);

  const endRoomDrag = useCallback(async () => {
    const st = dragRoomRef.current;
    if (!st) return;
    dragRoomRef.current = null;

    if (st.moved) {
      suppressRoomClickRef.current = true;
      setTimeout(() => {
        suppressRoomClickRef.current = false;
      }, 0);
    }

    const room = rooms.find(r => r.id === st.roomId);
    if (!room) return;
    await persistRoomGeometry(room.id, room.points, room.wallLengths);
  }, [persistRoomGeometry, rooms]);

  const handleRoomClick = useCallback((roomId) => {
    if (suppressRoomClickRef.current) {
      suppressRoomClickRef.current = false;
      return;
    }
    setActiveRoomId(roomId);
    setActiveWallIndex(null);
    setActiveVertexIndex(null);
  }, []);

  const beginRoomVertexDrag = useCallback((e, roomId, vertexIndex) => {
    if (!editEnabled) return;
    if (mode === 'draw') return;
    if (!roomId) return;
    if (!Number.isFinite(vertexIndex)) return;
    if (e.button !== 0) return;

    const room = rooms.find(r => r.id === roomId);
    const pts = Array.isArray(room?.points) ? room.points : [];
    if (pts.length < 3) return;
    if (vertexIndex < 0 || vertexIndex >= pts.length) return;

    e.stopPropagation();
    panRef.current.active = false;
    setActiveRoomId(roomId);
    setActiveWallIndex(null);
    setActiveVertexIndex(vertexIndex);

    dragRoomVertexRef.current = {
      roomId,
      vertexIndex,
      moved: false,
      startPoints: pts.map(pt => ({ x: pt.x, y: pt.y })),
    };
    suppressRoomClickRef.current = false;

    try {
      e.currentTarget?.setPointerCapture?.(e.pointerId);
    } catch {
      // ignore
    }
  }, [editEnabled, mode, rooms]);

  const handleRoomVertexDragMove = useCallback((e) => {
    const st = dragRoomVertexRef.current;
    if (!st) return false;

    const p0 = svgPointFromEvent(e);
    if (!p0) return true;

    let p = { ...p0, snapped: false, snapKind: '' };
    const original = st.startPoints[st.vertexIndex];

    if (snapSettings.grid) {
      const g = snapToGrid(p);
      if (g?.snapped) p = { ...g, snapKind: 'grid' };
    }

    if (snapSettings.align) {
      p = snapAxisToVertices(p, original);
    }

    // Do not vertex-snap while dragging a vertex (it can get "stuck" on itself).
    const geom = snapPoint(p, { vertex: false, edge: snapSettings.edge });
    if (geom?.snapped) {
      p = { ...geom, snapKind: 'geom' };
    }

    const moved = Math.abs(p.x - original.x) + Math.abs(p.y - original.y) > 0.25;
    if (moved) st.moved = true;

    applyEditorUpdateBatched(`vertex-drag:${st.roomId}:${st.vertexIndex}`, prev => {
      const nextRooms = (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => {
        if (r.id !== st.roomId) return r;
        const pts = st.startPoints.map(pt => ({ x: pt.x, y: pt.y }));
        pts[st.vertexIndex] = { x: p.x, y: p.y };
        return { ...r, points: pts };
      });
      return { ...prev, rooms: nextRooms };
    });

    return true;
  }, [applyEditorUpdateBatched, snapAxisToVertices, snapPoint, snapSettings.align, snapSettings.edge, snapSettings.grid, snapToGrid, svgPointFromEvent]);

  const endRoomVertexDrag = useCallback(async () => {
    const st = dragRoomVertexRef.current;
    if (!st) return;
    dragRoomVertexRef.current = null;

    if (st.moved) {
      suppressRoomClickRef.current = true;
      setTimeout(() => {
        suppressRoomClickRef.current = false;
      }, 0);
    }

    const room = rooms.find(r => r.id === st.roomId);
    if (!room) return;
    await persistRoomGeometry(room.id, room.points, room.wallLengths);
  }, [persistRoomGeometry, rooms]);

  const zoomBy = useCallback((factor) => {
    const svg = svgRef.current;
    if (!svg) {
      setView(prev => ({ ...prev, scale: clamp((prev.scale || 1) * factor, MIN_ZOOM, MAX_ZOOM) }));
      return;
    }
    const rect = svg.getBoundingClientRect();
    const cx = rect.width / 2;
    const cy = rect.height / 2;
    setView(prev => {
      const currentScale = prev.scale || 1;
      const nextScale = clamp(currentScale * factor, MIN_ZOOM, MAX_ZOOM);
      const worldX = (cx - prev.tx) / currentScale;
      const worldY = (cy - prev.ty) / currentScale;
      const nextTx = cx - worldX * nextScale;
      const nextTy = cy - worldY * nextScale;
      return { scale: nextScale, tx: nextTx, ty: nextTy };
    });
  }, []);

  const fitViewToContent = useCallback(() => {
    const svg = svgRef.current;
    if (!svg) return false;

    const rect = svg.getBoundingClientRect();
    const w = rect.width;
    const h = rect.height;
    if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 10 || h <= 10) return false;

    const bounds = computeWorldBoundsFromLayout(rooms, layout.devicePlacements);
    if (!bounds) return false;

    const padding = 64;
    const bw = Math.max(1, bounds.maxX - bounds.minX);
    const bh = Math.max(1, bounds.maxY - bounds.minY);

    const scaleX = (w - padding * 2) / bw;
    const scaleY = (h - padding * 2) / bh;
    const nextScale = clamp(Math.min(scaleX, scaleY), MIN_ZOOM, MAX_ZOOM);

    const cxWorld = (bounds.minX + bounds.maxX) / 2;
    const cyWorld = (bounds.minY + bounds.maxY) / 2;
    const cxScreen = w / 2;
    const cyScreen = h / 2;
    const nextTx = cxScreen - cxWorld * nextScale;
    const nextTy = cyScreen - cyWorld * nextScale;

    setView({ scale: nextScale, tx: nextTx, ty: nextTy });
    return true;
  }, [layout.devicePlacements, rooms]);

  const resetView = useCallback(() => {
    if (!fitViewToContent()) {
      setView({ scale: 1, tx: 0, ty: 0 });
    }
  }, [fitViewToContent]);

  useEffect(() => {
    if (didAutoCenterRef.current) return;
    if (!rooms.length) return;
    const ok = fitViewToContent();
    if (ok) didAutoCenterRef.current = true;
  }, [fitViewToContent, rooms.length]);

  const setWallLength = useCallback((roomId, wallIndex, length) => {
    applyEditorUpdate(prev => ({
      ...prev,
      rooms: (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => {
        if (r.id !== roomId) return r;
        const current = Array.isArray(r.wallLengths) ? r.wallLengths.slice() : [];
        while (current.length < (Array.isArray(r.points) ? r.points.length : 0)) {
          current.push(null);
        }
        current[wallIndex] = length;
        const pts = applyLengthToSegment(r.points, wallIndex, length);
        return { ...r, wallLengths: current, points: pts };
      }),
    }));
  }, [applyEditorUpdate]);

  const setDraftWallLength = useCallback((wallIndex, length) => {
    setDraft(prev => {
      if (!prev) return prev;
      const current = Array.isArray(prev.wallLengths) ? prev.wallLengths.slice() : [];
      while (current.length < (Array.isArray(prev.points) ? prev.points.length : 0)) {
        current.push(null);
      }
      current[wallIndex] = length;
      const pts = applyLengthToSegment(prev.points, wallIndex, length);
      return { ...prev, wallLengths: current, points: pts };
    });
  }, []);

  const persistDevicePlacement = useCallback(async (deviceKey, roomId, x, y) => {
    const dev = deviceByKey.get(deviceKey);
    const ersId = safeString(dev?.ersId);
    if (!ersId || !accessToken) return;
    await patchErsDevice(ersId, {
      room_id: roomId || null,
      meta: { map: { x, y } },
    }, accessToken);
  }, [accessToken, deviceByKey]);

  const persistDeviceFavoriteFields = useCallback(async (deviceKey, favoriteFields) => {
    const dev = deviceByKey.get(deviceKey);
    const ersId = safeString(dev?.ersId);
    if (!ersId || !accessToken) return;
    const list = Array.isArray(favoriteFields) ? favoriteFields : [];
    const normalized = list
      .map(v => safeString(v).trim())
      .filter(Boolean);
    const deduped = Array.from(new Set(normalized));
    await patchErsDevice(ersId, {
      meta: { map: { favorite_fields: deduped.length ? deduped : null } },
    }, accessToken);
    if (typeof refreshErsInventory === 'function') {
      refreshErsInventory();
    }
  }, [accessToken, deviceByKey, refreshErsInventory]);

  const removeDeviceFromMap = useCallback(async (deviceKey) => {
    if (!deviceKey) return;

    setLayout(prev => {
      const nextPlacements = { ...(prev.devicePlacements || {}) };
      delete nextPlacements[deviceKey];
      return { ...prev, devicePlacements: nextPlacements };
    });
    setExpandedDeviceKey(prev => (prev === deviceKey ? '' : prev));

    const dev = deviceByKey.get(deviceKey);
    const ersId = safeString(dev?.ersId);
    if (!ersId || !accessToken) return;

    await patchErsDevice(ersId, {
      room_id: null,
      meta: { map: null },
    }, accessToken);
  }, [accessToken, deviceByKey, setLayout]);

  const assignDeviceToRoomAt = useCallback(async (deviceKey, roomId, point) => {
    if (!deviceKey || !roomId || !point) return;
    setLayout(prev => ({
      ...prev,
      devicePlacements: {
        ...(prev.devicePlacements || {}),
        [deviceKey]: { roomId, x: point.x, y: point.y },
      },
    }));

    await persistDevicePlacement(deviceKey, roomId, point.x, point.y);
  }, [persistDevicePlacement, setLayout]);

  const roomAtPoint = useCallback((p) => {
    if (!p) return null;
    for (const r of rooms) {
      if (pointInPolygon(p, r.points)) return r;
    }
    return null;
  }, [rooms]);

  const beginDeviceDrag = useCallback((e, devKey) => {
    if (!editEnabled) return;
    if (mode === 'draw') return;
    if (!devKey) return;
    if (e.button !== 0) return;

    const placement = layout.devicePlacements?.[devKey];
    if (!placement || typeof placement !== 'object') return;

    const startX = Number(placement.x);
    const startY = Number(placement.y);
    if (!Number.isFinite(startX) || !Number.isFinite(startY)) return;

    const p = svgPointFromEvent(e);
    if (!p) return;

    e.stopPropagation();

    dragDeviceRef.current = {
      key: devKey,
      offsetX: p.x - startX,
      offsetY: p.y - startY,
      lastX: startX,
      lastY: startY,
      moved: false,
    };

    suppressDeviceClickRef.current = false;

    try {
      e.currentTarget?.setPointerCapture?.(e.pointerId);
    } catch {
      // ignore
    }
  }, [editEnabled, layout.devicePlacements, mode, svgPointFromEvent]);

  const handleDeviceDragMove = useCallback((e) => {
    const st = dragDeviceRef.current;
    if (!st) return;
    const p = svgPointFromEvent(e);
    if (!p) return;

    const x = p.x - st.offsetX;
    const y = p.y - st.offsetY;
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;

    if (Math.abs(x - st.lastX) + Math.abs(y - st.lastY) > 0.25) st.moved = true;
    st.lastX = x;
    st.lastY = y;

    const r = roomAtPoint({ x, y });
    const roomId = safeString(r?.id);

    setLayout(prev => ({
      ...prev,
      devicePlacements: {
        ...(prev.devicePlacements || {}),
        [st.key]: {
          ...(prev.devicePlacements?.[st.key] || {}),
          x,
          y,
          roomId: roomId || prev.devicePlacements?.[st.key]?.roomId || '',
        },
      },
    }));
  }, [roomAtPoint, setLayout, svgPointFromEvent]);

  const endDeviceDrag = useCallback(async () => {
    const st = dragDeviceRef.current;
    if (!st) return;
    dragDeviceRef.current = null;

    if (st.moved) {
      suppressDeviceClickRef.current = true;
      setTimeout(() => {
        suppressDeviceClickRef.current = false;
      }, 0);
    }

    const r = roomAtPoint({ x: st.lastX, y: st.lastY });
    const roomId = safeString(r?.id);
    await persistDevicePlacement(st.key, roomId, st.lastX, st.lastY);
  }, [persistDevicePlacement, roomAtPoint]);

  const deleteRoom = useCallback(async (roomId) => {
    if (!roomId) return;
    if (!accessToken) {
      setMapError('Authentication required');
      return;
    }
    setOpPending(true);
    setMapError('');
    try {
      const res = await deleteErsRoom(roomId, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to delete room');
      setLayout(prev => {
        const nextRooms = (Array.isArray(prev.rooms) ? prev.rooms : []).filter(r => r.id !== roomId);
        const nextPlacements = { ...(prev.devicePlacements || {}) };
        Object.keys(nextPlacements).forEach(k => {
          if (nextPlacements[k]?.roomId === roomId) delete nextPlacements[k];
        });
        return { ...prev, rooms: nextRooms, devicePlacements: nextPlacements };
      });
      setActiveRoomId('');
      setActiveWallIndex(null);
    } catch (err) {
      setMapError(err?.message || 'Unable to delete room');
    } finally {
      setOpPending(false);
    }
  }, [accessToken, setLayout]);

  const closestSegmentHit = useCallback((p) => {
    let best = null; // { roomId, segIndex, point, d }
    rooms.forEach(r => {
      const pts = Array.isArray(r.points) ? r.points : [];
      if (pts.length < 2) return;
      for (let i = 0; i < pts.length; i += 1) {
        const a = pts[i];
        const b = pts[(i + 1) % pts.length];
        const { point } = closestPointOnSegment(p, a, b);
        const d = dist(p, point);
        if (!best || d < best.d) {
          best = { roomId: r.id, segIndex: i, point, d };
        }
      }
    });
    if (best && best.d <= snapWorld) return best;
    return null;
  }, [rooms, snapWorld]);

  const insertCornerOnRoom = useCallback((roomId, segIndex, point) => {
    if (!roomId || !point) return;
    const before = layoutRef.current;
    const currentRoom = (Array.isArray(before?.rooms) ? before.rooms : []).find(r => r.id === roomId);
    const currentPts = Array.isArray(currentRoom?.points) ? currentRoom.points : [];
    if (currentPts.length < 2) return;
    const insertAt = clamp(segIndex + 1, 1, currentPts.length);
    const nextPts = currentPts.slice();
    nextPts.splice(insertAt, 0, { x: point.x, y: point.y });
    const wl = Array.isArray(currentRoom?.wallLengths) ? currentRoom.wallLengths.slice() : [];
    wl.splice(insertAt - 1, 0, null);

    applyEditorUpdate(prev => {
      const nextRooms = (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => {
        if (r.id !== roomId) return r;
        return { ...r, points: nextPts, wallLengths: wl };
      });
      return { ...prev, rooms: nextRooms };
    });

    setActiveRoomId(roomId);
    setActiveWallIndex(segIndex);
    setActiveVertexIndex(insertAt);
    void persistRoomGeometry(roomId, nextPts, wl);
  }, [applyEditorUpdate, layoutRef, persistRoomGeometry]);

  const deleteCornerOnRoom = useCallback((roomId, vertexIndex) => {
    if (!roomId) return;
    if (!Number.isFinite(vertexIndex)) return;

    const before = layoutRef.current;
    const currentRoom = (Array.isArray(before?.rooms) ? before.rooms : []).find(r => r.id === roomId);
    const pts = Array.isArray(currentRoom?.points) ? currentRoom.points : [];
    if (pts.length <= 3) {
      setMapError('A room needs at least 3 corners');
      return;
    }
    if (vertexIndex < 0 || vertexIndex >= pts.length) return;

    const n = pts.length;
    const nextPts = pts.slice();
    nextPts.splice(vertexIndex, 1);

    const rawWl = Array.isArray(currentRoom?.wallLengths) ? currentRoom.wallLengths.slice() : [];
    const oldWl = [];
    for (let i = 0; i < n; i += 1) oldWl.push(rawWl[i] ?? null);

    const m = nextPts.length;
    const oldIndexFromNew = (i) => (i < vertexIndex ? i : i + 1);
    const nextWl = [];
    for (let i = 0; i < m; i += 1) {
      const oldStart = oldIndexFromNew(i);
      const oldEnd = oldIndexFromNew((i + 1) % m);
      const isContiguous = oldEnd === ((oldStart + 1) % n);
      nextWl.push(isContiguous ? oldWl[oldStart] : null);
    }

    applyEditorUpdate(prev => {
      const nextRooms = (Array.isArray(prev.rooms) ? prev.rooms : []).map(r => {
        if (r.id !== roomId) return r;
        return { ...r, points: nextPts, wallLengths: nextWl };
      });
      return { ...prev, rooms: nextRooms };
    });

    setActiveRoomId(roomId);
    setActiveWallIndex(null);
    setActiveVertexIndex(null);
    void persistRoomGeometry(roomId, nextPts, nextWl);
  }, [applyEditorUpdate, layoutRef, persistRoomGeometry, setMapError]);

  useEffect(() => {
    if (!editEnabled) return;

    const isTypingTarget = (t) => {
      if (!t || typeof t !== 'object') return false;
      const tag = (t.tagName || '').toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
      return Boolean(t.isContentEditable);
    };

    const onKeyDown = (e) => {
      if (isTypingTarget(e.target)) return;

      const key = String(e.key || '').toLowerCase();
      const mod = e.metaKey || e.ctrlKey;

      if (mod && key === 'z' && !e.shiftKey) {
        if (!canUndo) return;
        e.preventDefault();
        handleUndo();
        return;
      }

      if (mod && (key === 'y' || (key === 'z' && e.shiftKey))) {
        if (!canRedo) return;
        e.preventDefault();
        handleRedo();
        return;
      }

      if ((e.key === 'Backspace' || e.key === 'Delete') && mode !== 'draw') {
        if (!activeRoomId) return;
        if (!Number.isFinite(activeVertexIndex)) return;
        e.preventDefault();
        deleteCornerOnRoom(activeRoomId, activeVertexIndex);
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [activeRoomId, activeVertexIndex, canRedo, canUndo, deleteCornerOnRoom, editEnabled, handleRedo, handleUndo, mode]);

  const beginInsertCornerDrag = useCallback((e, roomId, segIndex) => {
    if (!editEnabled) return;
    if (mode === 'draw') return;
    if (!roomId) return;
    if (!Number.isFinite(segIndex)) return;
    if (e.button !== 0) return;

    const room = rooms.find(r => r.id === roomId);
    const pts = Array.isArray(room?.points) ? room.points : [];
    if (pts.length < 3) return;
    if (segIndex < 0 || segIndex >= pts.length) return;

    e.stopPropagation();
    panRef.current.active = false;
    setActiveRoomId(roomId);
    setActiveWallIndex(segIndex);

    const a = pts[segIndex];
    const b = pts[(segIndex + 1) % pts.length];
    if (a && b) {
      setInsertCornerPreview({ roomId, segIndex, point: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } });
    }

    dragInsertCornerRef.current = { roomId, segIndex, moved: false };
    suppressRoomClickRef.current = false;

    try {
      e.currentTarget?.setPointerCapture?.(e.pointerId);
    } catch {
      // ignore
    }
  }, [editEnabled, mode, rooms]);

  const handleInsertCornerDragMove = useCallback((e) => {
    const st = dragInsertCornerRef.current;
    if (!st) return false;

    const p0 = svgPointFromEvent(e);
    if (!p0) return true;

    let p = { ...p0, snapped: false, snapKind: '' };

    if (snapSettings.grid) {
      const g = snapToGrid(p);
      if (g?.snapped) p = { ...g, snapKind: 'grid' };
    }

    if (snapSettings.align) {
      const axis = snapAxisToVertices(p);
      if (axis?.snapped) p = { ...axis, snapKind: axis.snapKind || 'axis' };
    }

    // Do not edge-snap while creating a new corner (it can feel "sticky" to walls).
    const geom = snapPoint(p, { vertex: snapSettings.vertex, edge: false });
    if (geom?.snapped) {
      p = { ...geom, snapKind: 'geom' };
    }

    st.moved = true;
    setInsertCornerPreview({ roomId: st.roomId, segIndex: st.segIndex, point: { x: p.x, y: p.y } });
    return true;
  }, [snapAxisToVertices, snapPoint, snapSettings.align, snapSettings.grid, snapSettings.vertex, snapToGrid, svgPointFromEvent]);

  const endInsertCornerDrag = useCallback(() => {
    const st = dragInsertCornerRef.current;
    if (!st) return;
    dragInsertCornerRef.current = null;

    const preview = insertCornerPreview;
    setInsertCornerPreview(null);

    if (preview?.roomId === st.roomId && preview?.segIndex === st.segIndex && preview?.point) {
      insertCornerOnRoom(st.roomId, st.segIndex, preview.point);
    }

    suppressRoomClickRef.current = true;
    setTimeout(() => {
      suppressRoomClickRef.current = false;
    }, 0);
  }, [insertCornerOnRoom, insertCornerPreview]);

  const handleCanvasPointerMove = useCallback((e) => {
    if (mode !== 'draw') return;
    const p = svgPointFromEvent(e);
    if (!p) return;
    const pts = Array.isArray(draft?.points) ? draft.points : [];
    const prevPoint = pts.length > 0 ? pts[pts.length - 1] : null;
    const firstPoint = pts.length >= 3 ? pts[0] : null;
    const { point: snappedPoint, guide } = snapForDraw(prevPoint, p, firstPoint);
    setHoverPoint(snappedPoint);
    setSnapGuide(guide);
  }, [draft?.points, mode, snapForDraw, svgPointFromEvent]);

  const {
    handlePointerDown,
    handleCanvasPointerMoveCombined,
    handlePointerUp,
  } = useMapViewportHandlers({
    svgRef,
    canvasRef,
    mode,
    view,
    setView,
    viewRef,
    panRef,
    minZoom: MIN_ZOOM,
    maxZoom: MAX_ZOOM,
    handleInsertCornerDragMove,
    handleRoomVertexDragMove,
    handleRoomDragMove,
    handleDeviceDragMove,
    endDeviceDrag,
    endRoomDrag,
    endRoomVertexDrag,
    endInsertCornerDrag,
    handleCanvasPointerMove,
  });

  const handleContextMenu = useCallback((e) => {
    e.preventDefault();
    if (mode !== 'draw') {
      // cancel device selection on right click
      if (selectedDeviceId) setSelectedDeviceId('');
      return;
    }
    setDraft(prev => {
      if (!prev) return prev;
      const pts = Array.isArray(prev.points) ? prev.points.slice() : [];
      if (pts.length === 0) {
        return prev;
      }
      pts.pop();
      return { ...prev, points: pts };
    });
    setHoverPoint(null);
    setActiveWallIndex(idx => (typeof idx === 'number' ? Math.max(0, idx - 1) : idx));
  }, [mode, selectedDeviceId]);

  const handleCanvasClick = useCallback((e) => {
    // if this click was actually a pan drag, ignore
    if (panRef.current.moved) {
      panRef.current.moved = false;
      return;
    }

    if (!editEnabled) {
      setExpandedDeviceKey('');
      return;
    }

    const p0 = svgPointFromEvent(e);
    if (!p0) return;
    let p = snapPoint(p0, { vertex: snapSettings.vertex, edge: snapSettings.edge });

    // Mobile click-to-place device
    if (mode === 'select' && selectedDeviceId) {
      const r = roomAtPoint(p);
      if (r) {
        assignDeviceToRoomAt(selectedDeviceId, r.id, p);
        setSelectedDeviceId('');
      }
      return;
    }

    if (mode !== 'draw') {
      // If clicking near an existing wall, insert a corner.
      const hit = closestSegmentHit(p);
      if (hit) {
        insertCornerOnRoom(hit.roomId, hit.segIndex, hit.point);
        return;
      }
      const r = roomAtPoint(p);
      setActiveRoomId(r?.id || '');
      setActiveWallIndex(null);
      setActiveVertexIndex(null);
      return;
    }

    const ptsSoFar = Array.isArray(draft?.points) ? draft.points : [];
    if (p) {
      const prevPoint = ptsSoFar.length > 0 ? ptsSoFar[ptsSoFar.length - 1] : null;
      const firstPoint = ptsSoFar.length >= 3 ? ptsSoFar[0] : null;
      const snapped = snapForDraw(prevPoint, p, firstPoint);
      if (snapped?.point) p = snapped.point;
      setSnapGuide(snapped?.guide || null);
    }

    setDraft(prev => {
      if (!prev) return prev;
      const pts = Array.isArray(prev.points) ? prev.points.slice() : [];
      if (pts.length >= 3) {
        const first = pts[0];
        if (dist(p, first) <= snapWorld) {
          // close polygon
          return { ...prev, points: pts };
        }
      }
      pts.push({ x: p.x, y: p.y });
      return { ...prev, points: pts };
    });

    // update active wall index after adding point
    setDraft(prev => {
      if (!prev) return prev;
      const pts = Array.isArray(prev.points) ? prev.points : [];
      if (pts.length < 2) return prev;
      setActiveWallIndex(pts.length - 2);
      return prev;
    });
  }, [assignDeviceToRoomAt, closestSegmentHit, draft?.points, editEnabled, insertCornerOnRoom, mode, roomAtPoint, selectedDeviceId, snapForDraw, snapPoint, snapSettings.edge, snapSettings.vertex, snapWorld, svgPointFromEvent]);

  useEffect(() => {
    if (mode !== 'draw') setSnapGuide(null);
  }, [mode]);

  const handleDrop = useCallback((e) => {
    if (!editEnabled) return;
    e.preventDefault();
    const deviceKey = e.dataTransfer.getData('text/homenavi-device-key');
    if (!deviceKey) return;
    const p0 = svgPointFromEvent(e);
    if (!p0) return;
    const p = snapPoint(p0, { vertex: snapSettings.vertex, edge: snapSettings.edge });
    const r = roomAtPoint(p);
    if (!r) return;
    assignDeviceToRoomAt(deviceKey, r.id, p);
  }, [assignDeviceToRoomAt, editEnabled, roomAtPoint, snapPoint, snapSettings.edge, snapSettings.vertex, svgPointFromEvent]);

  const preventDefault = useCallback((e) => {
    e.preventDefault();
  }, []);

  const roomPaths = useMemo(() => rooms.map(r => ({ ...r, path: roomPolygonToPath(r.points) })), [rooms]);

  const draftPath = useMemo(() => {
    if (!draft || !Array.isArray(draft.points) || draft.points.length === 0) return '';
    const pts = draft.points.slice();
    if (hoverPoint && pts.length >= 1) {
      pts.push({ x: hoverPoint.x, y: hoverPoint.y });
    }
    return pts.length >= 2 ? `M ${pts.map(p => `${p.x} ${p.y}`).join(' L ')}` : '';
  }, [draft, hoverPoint]);

  const activeWallDisplay = useMemo(() => {
    const r = activeRoom;
    if (!r) return null;
    const pts = Array.isArray(r.points) ? r.points : [];
    const segLens = computeSegmentLengths(pts);

    const wallIndex = activeWallIndex;
    if (wallIndex === null || wallIndex === undefined) return null;
    if (wallIndex < 0 || wallIndex >= segLens.length) return null;

    const stored = Array.isArray(r.wallLengths) ? r.wallLengths[wallIndex] : null;
    const fallback = segLens[wallIndex];
    const value = Number.isFinite(stored) ? stored : fallback;
    return {
      wallIndex,
      value,
    };
  }, [activeRoom, activeWallIndex]);

  const busy = (ersLoading || realtimeLoading) && devicesForPalette.length === 0;

  const expandedDevice = useMemo(() => {
    if (!expandedDeviceKey) return null;
    const placement = layout.devicePlacements?.[expandedDeviceKey];
    const dev = deviceByKey.get(expandedDeviceKey)
      || devicesForPalette.find(d => safeString(d?.id || d?.hdpId || d?.ersId) === expandedDeviceKey);
    if (!dev && !placement) return null;
    const displayName = safeString(dev?.displayName || dev?.name || expandedDeviceKey);
    const roomId = safeString(placement?.roomId);
    const roomName = roomId ? safeString(rooms.find(r => r.id === roomId)?.name) : '';
    const tagList = Array.isArray(dev?.tags) ? dev.tags : [];
    const tags = tagList.map(t => safeString(t)).filter(Boolean).slice(0, 6);
    // Device details route is keyed by the realtime device id when available.
    const deviceRouteId = safeString(dev?.hdpId || dev?.id || dev?.ersId || expandedDeviceKey);

    const x = Number(placement?.x);
    const y = Number(placement?.y);
    const hasXY = Number.isFinite(x) && Number.isFinite(y);
    const screenX = hasXY ? (x * (view.scale || 1) + (view.tx || 0)) : null;
    const screenY = hasXY ? (y * (view.scale || 1) + (view.ty || 0)) : null;

    const state = dev?.state && typeof dev.state === 'object' ? dev.state : {};
    const allCaps = [
      ...(Array.isArray(dev?.capabilities) ? dev.capabilities : []),
      ...(Array.isArray(dev?.state?.capabilities) ? dev.state.capabilities : []),
    ];
    const favoriteFields = readFavoriteFieldsFromErsMeta(dev);
    const favoriteOptions = collectFavoriteFieldOptionsFromState(state);
    const quickFields = [
      { key: 'state', label: 'State' },
      { key: 'on', label: 'Power' },
      { key: 'temperature', label: 'Temp' },
      { key: 'humidity', label: 'Humidity' },
      { key: 'battery', label: 'Battery' },
      { key: 'power', label: 'Power' },
      { key: 'contact', label: 'Contact' },
      { key: 'motion', label: 'Motion' },
      { key: 'brightness', label: 'Brightness' },
    ];
    const facts = [];

    favoriteFields.forEach((key) => {
      if (!key) return;
      const raw = pickStateValue(state, key);
      const { valueText, unit } = formatMetricValueAndUnitForKey(key, raw, allCaps);
      const v = valueText ? `${valueText}${unit || ''}` : '';
      if (!v) return;
      if (!facts.some(existing => existing.label === key)) {
        facts.push({ key, label: key, value: v, favorite: true });
      }
    });

    for (const f of quickFields) {
      const raw = pickStateValue(state, f.key);
      const { valueText, unit } = formatMetricValueAndUnitForKey(f.key, raw, allCaps);
      const v = valueText ? `${valueText}${unit || ''}` : '';
      if (!v) continue;
      if (!facts.some(existing => existing.label === f.label)) {
        facts.push({ key: f.key, label: f.label, value: v });
      }
      if (facts.length >= 4) break;
    }
    const lastSeen = dev?.lastSeen instanceof Date ? dev.lastSeen : (dev?.stateUpdatedAt instanceof Date ? dev.stateUpdatedAt : null);
    const lastSeenText = lastSeen ? formatRelativeTimeShort(lastSeen) : '';

    return {
      key: expandedDeviceKey,
      displayName,
      roomName,
      tags,
      deviceRouteId,
      online: Boolean(dev?.online),
      facts,
      lastSeenText,
      favoriteFields,
      favoriteOptions,
      screenX,
      screenY,
    };
  }, [deviceByKey, devicesForPalette, expandedDeviceKey, layout.devicePlacements, rooms, view.scale, view.tx, view.ty]);

  const renderPlacedDevices = useCallback(() => (
    Object.entries(layout.devicePlacements || {}).map(([devKey, placement]) => {
      if (!placement || typeof placement !== 'object') return null;
      const x = Number(placement.x);
      const y = Number(placement.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
      const dev = deviceByKey.get(devKey) || devicesForPalette.find(d => safeString(d?.id || d?.hdpId || d?.ersId) === devKey);
      const label = safeString(dev?.displayName || dev?.name || devKey);

      const state = dev?.state && typeof dev.state === 'object' ? dev.state : null;
      const favorites = readFavoriteFieldsFromErsMeta(dev);
      const allCaps = [
        ...(Array.isArray(dev?.capabilities) ? dev.capabilities : []),
        ...(Array.isArray(dev?.state?.capabilities) ? dev.state.capabilities : []),
      ];
      const favoriteLines = favorites
        .map((favoriteKey) => {
          const favoriteRaw = favoriteKey && state ? pickStateValue(state, favoriteKey) : undefined;
          const { valueText, unit } = formatMetricValueAndUnitForKey(favoriteKey, favoriteRaw, allCaps);
          const text = valueText ? `${valueText}${unit || ''}` : '';
          return {
            key: favoriteKey,
            text,
            icon: iconForFactLabel(favoriteKey),
          };
        })
        .filter(x => x && x.text);

      return (
        <g
          key={`dev-${devKey}`}
          className={`map-device-group${editEnabled ? ' editable' : ''}`}
          onPointerDown={(e) => {
            if (!editEnabled) return;
            beginDeviceDrag(e, devKey);
          }}
          onClick={(e) => {
            e.stopPropagation();
            if (suppressDeviceClickRef.current) {
              suppressDeviceClickRef.current = false;
              return;
            }
            setExpandedDeviceKey(prev => (prev === devKey ? '' : devKey));
          }}
          title={editEnabled ? 'Drag to reposition, click for details' : 'Click for details'}
        >
          <circle cx={x} cy={y} r={5} className="map-device" />
          <text x={x + 8} y={y + 4} className="map-label">{label}</text>
          {favoriteLines.length ? (
            <>
              {favoriteLines.map((line, idx) => {
                const path = line.icon ? getFaSvgPath(line.icon) : null;
                const iconX = x + 8;
                const iconY = y + 8 + (idx * 14);
                const textX = x + 8 + 16;
                const textY = y + 18 + (idx * 14);
                return (
                  <Fragment key={`${devKey}-fav-${line.key}-${idx}`}>
                    {path ? (
                      <g transform={`translate(${iconX} ${iconY}) scale(${12 / path.height})`} aria-hidden="true">
                        <path className="map-fav-icon" d={path.path} />
                      </g>
                    ) : null}
                    <text x={textX} y={textY} className="map-label">{line.text}</text>
                  </Fragment>
                );
              })}
            </>
          ) : null}
        </g>
      );
    })
  ), [beginDeviceDrag, deviceByKey, devicesForPalette, editEnabled, layout.devicePlacements]);

  return {
    // auth/permissions
    isResidentOrAdmin,
    bootstrapping,

    // data
    ersError,
    ersLoading,
    realtimeLoading,
    busy,
    devicesForPalette,

    // refs
    svgRef,
    canvasRef,

    // general state
    editEnabled,
    setEditEnabled,
    mode,
    setMode,
    view,
    setView,
    viewRef,
    snapSettings,
    setSnapSettings,

    // room state
    rooms,
    roomPaths,
    activeRoomId,
    setActiveRoomId,
    activeRoom,
    roomNameEdit,
    setRoomNameEdit,
    activeWallIndex,
    setActiveWallIndex,
    activeVertexIndex,
    setActiveVertexIndex,
    activeWallDisplay,

    // drawing
    draft,
    setDraft,
    hoverPoint,
    setHoverPoint,
    draftPath,
    snapGuide,
    insertCornerPreview,

    // devices on map
    selectedDeviceId,
    setSelectedDeviceId,
    expandedDevice,
    expandedDeviceKey,
    setExpandedDeviceKey,
    favoritesEditorKey,
    setFavoritesEditorKey,

    // errors/ops
    mapError,
    setMapError,
    opPending,

    // history
    canUndo,
    canRedo,
    handleUndo,
    handleRedo,

    // helpers
    normalizeNumber,
    iconForFactLabel,
    navigate,

    // actions
    startRoom,
    cancelDraft,
    finalizeDraft,
    updateRoomName,
    deleteRoom,
    deleteCornerOnRoom,
    setWallLength,
    setDraftWallLength,
    persistRoomGeometry,
    persistDeviceFavoriteFields,
    removeDeviceFromMap,

    // event handlers
    zoomBy,
    resetView,
    handleCanvasPointerMoveCombined,
    handleCanvasClick,
    handlePointerDown,
    handlePointerUp,
    handleContextMenu,
    handleDrop,
    preventDefault,

    // svg handlers
    beginRoomDrag,
    beginRoomVertexDrag,
    beginInsertCornerDrag,
    onRoomClick: handleRoomClick,
    renderPlacedDevices,

    // constants
    gridSize: GRID_SIZE,
  };
}
