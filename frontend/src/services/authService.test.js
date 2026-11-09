import { describe, expect, it, vi } from 'vitest';
import http from './httpClient';
import { revokeOAuthConsent } from './authService';

vi.mock('./httpClient', () => ({ default: { del: vi.fn() } }));

describe('revokeOAuthConsent', () => {
  it('sends an authenticated DELETE with an encoded client ID', async () => {
    const response = { success: true };
    http.del.mockResolvedValueOnce(response);

    await expect(revokeOAuthConsent('client/with spaces', 'token')).resolves.toEqual(response);
    expect(http.del).toHaveBeenCalledWith('/api/auth/oauth/consents/client%2Fwith%20spaces', { token: 'token' });
  });
});