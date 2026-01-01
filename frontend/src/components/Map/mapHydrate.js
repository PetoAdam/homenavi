import { readDevicePlacementFromErsMeta, readRoomGeometryFromErsMeta, safeString } from './mapErsMeta';

export function mergeRoomsFromErs(prevLayout, ersRooms) {
  const prev = prevLayout && typeof prevLayout === 'object' ? prevLayout : {};
  if (!Array.isArray(ersRooms) || ersRooms.length === 0) return prev;

  const prevRooms = Array.isArray(prev.rooms) ? prev.rooms : [];
  const prevById = new globalThis.Map(prevRooms.map((r) => [safeString(r?.id), r]));

  const mergedRooms = [];
  ersRooms.forEach((room) => {
    const id = safeString(room?.id);
    if (!id) return;

    const local = prevById.get(id);
    const geom = readRoomGeometryFromErsMeta(room);

    const localPoints = Array.isArray(local?.points) ? local.points : [];
    const localWalls = Array.isArray(local?.wallLengths) ? local.wallLengths : [];

    const points = geom?.points || localPoints;
    const wallLengths = geom?.wallLengths || localWalls;

    mergedRooms.push({
      id,
      name: safeString(room?.name) || safeString(local?.name) || 'Room',
      points: Array.isArray(points) ? points : [],
      wallLengths: Array.isArray(wallLengths) ? wallLengths : [],
    });

    prevById.delete(id);
  });

  // Keep any purely-local rooms (should be rare).
  prevById.forEach((r) => mergedRooms.push(r));

  return { ...prev, rooms: mergedRooms };
}

export function mergeDevicePlacementsFromErs(prevLayout, devicesForPalette, draggingDeviceKey) {
  const prev = prevLayout && typeof prevLayout === 'object' ? prevLayout : {};
  if (!Array.isArray(devicesForPalette) || devicesForPalette.length === 0) return prev;

  const prevPlacements = prev.devicePlacements && typeof prev.devicePlacements === 'object' ? prev.devicePlacements : {};
  let changed = false;
  const nextPlacements = { ...prevPlacements };

  devicesForPalette.forEach((d) => {
    const key = safeString(d?.ersId || d?.id || d?.hdpId);
    if (!key) return;

    // Avoid fighting the pointer while dragging a device.
    if (draggingDeviceKey && draggingDeviceKey === key) return;

    const existing = nextPlacements[key];
    const placement = readDevicePlacementFromErsMeta(d);

    const hasXY = existing && Number.isFinite(Number(existing.x)) && Number.isFinite(Number(existing.y));
    if (!placement && hasXY) return;
    if (!placement) return;

    const roomId = safeString(d?.room_id) || safeString(existing?.roomId);
    if (!existing || Number(existing.x) !== placement.x || Number(existing.y) !== placement.y || safeString(existing.roomId) !== roomId) {
      nextPlacements[key] = { roomId, x: placement.x, y: placement.y };
      changed = true;
    }
  });

  return changed ? { ...prev, devicePlacements: nextPlacements } : prev;
}
