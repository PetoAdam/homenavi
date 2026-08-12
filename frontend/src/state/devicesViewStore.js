import { create } from 'zustand';
import {
  loadDevicesListPrefs,
  normalizeDevicesListPrefs,
  saveDevicesListPrefs,
} from '../components/Devices/devicesListPrefs';

const initialPrefs = normalizeDevicesListPrefs(loadDevicesListPrefs());

function applyAndPersist(set, updater) {
  set((state) => {
    const nextPartial = typeof updater === 'function' ? updater(state) : updater;
    const nextState = { ...state, ...nextPartial };
    saveDevicesListPrefs(nextState);
    return nextPartial;
  });
}

export const useDevicesViewStore = create((set) => ({
  metadataMode: initialPrefs.metadataMode,
  groupByRoom: initialPrefs.groupByRoom,
  protocolFilter: initialPrefs.protocolFilter,
  roomFilter: initialPrefs.roomFilter,
  tagFilter: initialPrefs.tagFilter,
  searchTerm: initialPrefs.searchTerm,

  setMetadataMode: (nextValue) => {
    applyAndPersist(set, (state) => {
      const resolved = typeof nextValue === 'function' ? nextValue(state.metadataMode) : nextValue;
      const normalized = normalizeDevicesListPrefs({ ...state, metadataMode: resolved });
      return { metadataMode: normalized.metadataMode };
    });
  },
  setGroupByRoom: (nextValue) => {
    applyAndPersist(set, (state) => {
      const normalized = normalizeDevicesListPrefs({ ...state, groupByRoom: nextValue });
      return { groupByRoom: normalized.groupByRoom };
    });
  },
  setProtocolFilter: (nextValue) => {
    applyAndPersist(set, (state) => {
      const normalized = normalizeDevicesListPrefs({ ...state, protocolFilter: nextValue });
      return { protocolFilter: normalized.protocolFilter };
    });
  },
  setRoomFilter: (nextValue) => {
    applyAndPersist(set, (state) => {
      const normalized = normalizeDevicesListPrefs({ ...state, roomFilter: nextValue });
      return { roomFilter: normalized.roomFilter };
    });
  },
  setTagFilter: (nextValue) => {
    applyAndPersist(set, (state) => {
      const normalized = normalizeDevicesListPrefs({ ...state, tagFilter: nextValue });
      return { tagFilter: normalized.tagFilter };
    });
  },
  setSearchTerm: (nextValue) => {
    applyAndPersist(set, (state) => {
      const normalized = normalizeDevicesListPrefs({ ...state, searchTerm: nextValue });
      return { searchTerm: normalized.searchTerm };
    });
  },
}));
