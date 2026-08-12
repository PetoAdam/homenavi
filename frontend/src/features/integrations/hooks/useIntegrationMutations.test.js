import { describe, expect, it, vi } from 'vitest';
import { queryKeys } from '../../../state/queryKeys';
import {
  syncIntegrationCaches,
  unwrapIntegrationResponse,
} from './useIntegrationMutations';

describe('unwrapIntegrationResponse', () => {
  it('returns response data for successful calls', () => {
    expect(unwrapIntegrationResponse({ success: true, data: { ok: true } }, 'fallback')).toEqual({ ok: true });
  });

  it('throws with fallback message and detail on failure', () => {
    expect(() => unwrapIntegrationResponse(
      { success: false, error: 'Failed', data: { detail: 'boom' } },
      'fallback'
    )).toThrow('Failed (boom)');
  });

  it('throws with fallback message when api error is missing', () => {
    expect(() => unwrapIntegrationResponse({ success: false }, 'fallback')).toThrow('fallback');
  });
});

describe('syncIntegrationCaches', () => {
  it('invalidates root, registry, and marketplace query keys', async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined);
    const queryClient = { invalidateQueries };
    const params = { q: 'demo', page: 2, pageSize: 20 };

    await syncIntegrationCaches(queryClient, params);

    expect(invalidateQueries).toHaveBeenCalledTimes(3);
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: queryKeys.integrations.root });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: queryKeys.integrations.registry(params) });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: queryKeys.integrations.marketplace() });
  });
});
