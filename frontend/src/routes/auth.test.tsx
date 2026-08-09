import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { Login } from './Login';
import { Register } from './Register';

const login = vi.fn();
const registerHousehold = vi.fn();

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    loading: false,
    session: null,
    login,
    register: registerHousehold,
  }),
}));

function renderRoute(element: React.ReactNode) {
  return render(<MemoryRouter>{element}</MemoryRouter>);
}

describe('parent authentication screens', () => {
  beforeEach(() => {
    cleanup();
    login.mockReset();
    registerHousehold.mockReset();
  });

  it('validates login fields before calling the API', async () => {
    renderRoute(<Login />);
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(
      await screen.findByText('Enter a valid email address.'),
    ).toBeInTheDocument();
    expect(login).not.toHaveBeenCalled();
  });

  it('collects registration in two steps and submits approved defaults atomically', async () => {
    registerHousehold.mockResolvedValue({ actor: 'parent', parentMode: true });
    renderRoute(<Register />);
    await userEvent.type(
      screen.getByLabelText('Parent email'),
      'parent@example.com',
    );
    await userEvent.type(
      screen.getByLabelText('Password'),
      'long-secure-password',
    );
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(await screen.findByLabelText('Household name')).toBeInTheDocument();
    await userEvent.type(
      screen.getByLabelText('Household name'),
      'Santoso family',
    );
    await userEvent.click(
      screen.getByRole('button', { name: 'Create household' }),
    );

    expect(registerHousehold).toHaveBeenCalledWith({
      email: 'parent@example.com',
      password: 'long-secure-password',
      householdName: 'Santoso family',
      timezone: 'Asia/Jakarta',
      weekStartsOn: 'sunday',
    });
  });
});
