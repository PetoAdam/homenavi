import { beforeEach, describe, expect, it } from 'vitest';
import { useDevicesViewStore } from './devicesViewStore';

const DEFAULTS = {
  metadataMode: 'ws',
  groupByRoom: true,
  protocolFilter: 'all',
  roomFilter: 'all',
  tagFilter: 'all',
  searchTerm: '',
};

describe('useDevicesViewStore', () => {
  beforeEach(() => {
    useDevicesViewStore.setState({
      ...DEFAULTS,
    });
  });

  it('toggles metadata mode via updater function', () => {
    const { setMetadataMode } = useDevicesViewStore.getState();
    setMetadataMode((prev) => (prev === 'ws' ? 'rest' : 'ws'));
    expect(useDevicesViewStore.getState().metadataMode).toBe('rest');
  });

  it('toggles group by room via updater function', () => {
    const { setGroupByRoom } = useDevicesViewStore.getState();
    setGroupByRoom((prev) => !prev);
    expect(useDevicesViewStore.getState().groupByRoom).toBe(false);
  });

  it('normalizes invalid metadata mode to ws', () => {
    const { setMetadataMode } = useDevicesViewStore.getState();
    setMetadataMode('invalid');
    expect(useDevicesViewStore.getState().metadataMode).toBe('ws');
  });

  it('updates list filters and search term', () => {
    const state = useDevicesViewStore.getState();
    state.setProtocolFilter('zigbee');
    state.setRoomFilter('kitchen');
    state.setTagFilter('favorite');
    state.setSearchTerm('lamp');

    const next = useDevicesViewStore.getState();
    expect(next.protocolFilter).toBe('zigbee');
    expect(next.roomFilter).toBe('kitchen');
    expect(next.tagFilter).toBe('favorite');
    expect(next.searchTerm).toBe('lamp');
  });
});
