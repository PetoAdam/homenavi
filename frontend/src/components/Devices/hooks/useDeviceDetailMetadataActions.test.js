import { describe, expect, it } from 'vitest';
import {
  collectFavoriteFieldOptionsFromDevice,
  readFavoriteFieldsFromErsMeta,
} from './useDeviceDetailMetadataActions';

describe('readFavoriteFieldsFromErsMeta', () => {
  it('reads and deduplicates favorite fields from supported meta keys', () => {
    const result = readFavoriteFieldsFromErsMeta({
      meta: {
        map: {
          favorite_fields: [' temperature ', 'humidity', 'temperature'],
        },
      },
    });

    expect(result).toEqual(['temperature', 'humidity']);
  });

  it('falls back to single-key variants when arrays are missing', () => {
    const result = readFavoriteFieldsFromErsMeta({
      meta: {
        map: {
          favoriteKey: ' battery ',
        },
      },
    });

    expect(result).toEqual(['battery']);
  });
});

describe('collectFavoriteFieldOptionsFromDevice', () => {
  it('collects and sorts non-reserved state keys', () => {
    const result = collectFavoriteFieldOptionsFromDevice({
      state: {
        temperature: 21,
        schema: 'ignore',
        device_id: 'ignore',
        humidity: 55,
        battery: 88,
      },
    });

    expect(result).toEqual(['battery', 'humidity', 'temperature']);
  });

  it('returns an empty array when state is not available', () => {
    expect(collectFavoriteFieldOptionsFromDevice(null)).toEqual([]);
    expect(collectFavoriteFieldOptionsFromDevice({ state: [] })).toEqual([]);
  });
});
