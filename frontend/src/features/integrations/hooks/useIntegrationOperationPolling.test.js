import { describe, expect, it } from 'vitest';
import { collectActiveOperationIds } from './useIntegrationOperationPolling';

describe('collectActiveOperationIds', () => {
  it('returns unique ids across install, update, and restart maps', () => {
    const ids = collectActiveOperationIds({
      installing: { a: true, b: false },
      updating: { b: true, c: true },
      restarting: { a: true, d: true },
    });

    expect(ids).toHaveLength(4);
    expect(new Set(ids)).toEqual(new Set(['a', 'b', 'c', 'd']));
  });

  it('returns empty array when nothing is active', () => {
    expect(collectActiveOperationIds({
      installing: { a: false },
      updating: {},
      restarting: { b: false },
    })).toEqual([]);
  });

  it('handles missing maps safely', () => {
    expect(collectActiveOperationIds({})).toEqual([]);
  });
});
