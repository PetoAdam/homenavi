import { beforeEach, describe, expect, it, vi } from 'vitest';

const { post, setAccessToken } = vi.hoisted(() => ({
  post: vi.fn(),
  setAccessToken: vi.fn(),
}));

vi.mock('./httpClient', () => ({
  default: {
    post,
  },
  setAccessToken,
}));

import { bootstrapDemoSession } from './authService';

describe('authService demo bootstrap', () => {
  beforeEach(() => {
    post.mockReset();
    setAccessToken.mockReset();
  });

  it('returns login-shaped tokens and updates the shared access token cache', async () => {
    post.mockResolvedValue({
      success: true,
      data: {
        access_token: 'demo-access',
        refresh_token: 'demo-refresh',
      },
    });

    const result = await bootstrapDemoSession();

    expect(post).toHaveBeenCalledWith('/api/auth/demo/bootstrap', {});
    expect(setAccessToken).toHaveBeenCalledWith('demo-access');
    expect(result).toEqual({
      success: true,
      accessToken: 'demo-access',
      refreshToken: 'demo-refresh',
    });
  });
});