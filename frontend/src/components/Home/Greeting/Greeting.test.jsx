// @vitest-environment jsdom

import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import Greeting from './Greeting';

vi.mock('../../../context/AuthContext', () => ({
  useAuth: () => ({ user: { first_name: 'Alexandria' } }),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

it('keeps the complete greeting in the single-line text element', () => {
  const container = document.createElement('div');
  const root = createRoot(container);
  act(() => root.render(<Greeting showProfileTextButton />));
  expect(container.querySelector('.dashboard-greeting__text').textContent).toContain('Welcome back, Alexandria!');
  expect(container.firstChild.classList.contains('dashboard-greeting--with-profile-btn')).toBe(true);
  act(() => root.unmount());
});