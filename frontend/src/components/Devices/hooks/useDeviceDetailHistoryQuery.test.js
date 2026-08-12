import { describe, expect, it } from 'vitest';
import {
  buildHistoryRequest,
  isLatestHistoryRequest,
  normalizeHistoryPoints,
} from './useDeviceDetailHistoryQuery';

describe('buildHistoryRequest', () => {
  it('includes limit only when enabled', () => {
    const toRFC3339 = (value) => `iso:${value}`;

    const withLimit = buildHistoryRequest({
      fromLocal: '2026-08-12T00:00',
      toLocal: '2026-08-12T01:00',
      limitEnabled: true,
      limit: 150,
      order: 'desc',
      toRFC3339,
    });

    const withoutLimit = buildHistoryRequest({
      fromLocal: '2026-08-12T00:00',
      toLocal: '2026-08-12T01:00',
      limitEnabled: false,
      limit: 150,
      order: 'asc',
      toRFC3339,
    });

    expect(withLimit).toEqual({
      from: 'iso:2026-08-12T00:00',
      to: 'iso:2026-08-12T01:00',
      limit: 150,
      order: 'desc',
    });
    expect(withoutLimit).toEqual({
      from: 'iso:2026-08-12T00:00',
      to: 'iso:2026-08-12T01:00',
      limit: undefined,
      order: 'asc',
    });
  });
});

describe('normalizeHistoryPoints', () => {
  it('normalizes payload objects and JSON strings', () => {
    const normalized = normalizeHistoryPoints([
      { ts: '1', payload: { state: true }, retained: 1, topic: 'a' },
      { ts: '2', payload: '{"temperature":21.5}', retained: 0, topic: 'b' },
    ]);

    expect(normalized).toEqual([
      { ts: '1', payload: { state: true }, retained: true, topic: 'a' },
      { ts: '2', payload: { temperature: 21.5 }, retained: false, topic: 'b' },
    ]);
  });

  it('falls back to empty payload object for invalid payload values', () => {
    const normalized = normalizeHistoryPoints([
      { ts: '1', payload: '{bad json}', retained: false },
      { ts: '2', payload: null, retained: false },
      { ts: '3', payload: 123, retained: true, topic: 'c' },
      { ts: '4', payload: ['unexpected', 'array'], retained: true, topic: 'd' },
    ]);

    expect(normalized[0].payload).toEqual({});
    expect(normalized[1].payload).toEqual({});
    expect(normalized[2].payload).toEqual({});
    expect(normalized[3].payload).toEqual({});
    expect(normalized[2].retained).toBe(true);
    expect(normalized[2].topic).toBe('c');
  });
});

describe('isLatestHistoryRequest', () => {
  it('returns true only for the active request id', () => {
    expect(isLatestHistoryRequest(3, 3)).toBe(true);
    expect(isLatestHistoryRequest(2, 3)).toBe(false);
  });
});
