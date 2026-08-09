import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import { AppShell } from '../components/AppShell';
import { PagePlaceholder } from '../components/PagePlaceholder';
import { ProfilePicker } from './ProfilePicker';

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    loading: false,
    session: {
      actor: 'profile_picker',
      householdId: 'household-1',
      parentMode: false,
    },
    enterChild: vi.fn(),
    logout: vi.fn(),
  }),
}));

vi.mock('../api/client', () => ({
  profilesApi: { list: vi.fn().mockResolvedValue([]) },
}));

function renderRouter(initialPath = '/') {
  const router = createMemoryRouter(
    [
      { path: '/', element: <ProfilePicker /> },
      {
        path: '/parent',
        element: <AppShell />,
        children: [
          {
            index: true,
            element: (
              <PagePlaceholder
                title="Family overview"
                description="Test overview"
              />
            ),
          },
          {
            path: 'children',
            element: (
              <PagePlaceholder title="Children" description="Test children" />
            ),
          },
        ],
      },
    ],
    { initialEntries: [initialPath] },
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('frontend routing foundation', () => {
  it('shows accessible profile choices', async () => {
    renderRouter();

    expect(
      screen.getByRole('heading', { name: /who is using habit home/i }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('link', { name: /parent mode/i }),
    ).toHaveAttribute('href', '/parent/unlock');
  });

  it('resolves a nested parent route within the app shell', () => {
    renderRouter('/parent/children');

    expect(
      screen.getByRole('heading', { name: 'Children' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Children' })).toHaveClass(
      'active',
    );
  });
});
