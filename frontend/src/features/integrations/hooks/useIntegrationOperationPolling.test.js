import { describe, expect, it } from 'vitest';
import { collectActiveOperationIds, isCurrentOperationStatus } from './useIntegrationOperationPolling';

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

describe('isCurrentOperationStatus', () => {
  it('rejects a terminal status written before the current operation was queued', () => {
    expect(isCurrentOperationStatus(
      { stage: 'ready', updated_at: '2026-10-27T20:00:00.000Z' },
      Date.parse('2026-10-27T20:00:01.000Z')
    )).toBe(false);
  });

  it('accepts a status written after the current operation was queued', () => {
    expect(isCurrentOperationStatus(
      { stage: 'updating', updated_at: '2026-10-27T20:00:02.000Z' },
      Date.parse('2026-10-27T20:00:01.000Z')
    )).toBe(true);
  });
});
