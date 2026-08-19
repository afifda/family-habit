/* eslint-disable @typescript-eslint/unbound-method -- Vitest tracks the mocked object methods directly. */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  childrenApi,
  profileSummaryApi,
  profilesApi,
  type Child,
} from '../api/client';
import { RequireParent } from '../auth/RequireParent';
import { ChildShell } from './ChildShell';
import { Children } from './Children';
import { ProfilePicker } from './ProfilePicker';

const auth = vi.hoisted(() => ({
  enterChild: vi.fn(),
  leaveChild: vi.fn(),
  loading: false,
  session: {
    actor: 'profile_picker',
    householdId: 'household-1',
    childId: null as string | null,
    parentMode: false,
  },
}));

vi.mock('../auth/AuthProvider', () => ({ useAuth: () => auth }));
vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    profileSummaryApi: { get: vi.fn() },
    profilesApi: { list: vi.fn() },
    childrenApi: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      archive: vi.fn(),
    },
  };
});

const profile = {
  id: '11111111-1111-4111-8111-111111111111',
  nickname: 'Ari',
  avatar: 'fox' as const,
  color: '#F5B94C',
  pinRequired: true,
};

const child: Child = {
  id: profile.id,
  nickname: profile.nickname,
  avatar: profile.avatar,
  color: profile.color,
  pinEnabled: true,
  active: true,
  createdAt: '2026-08-09T00:00:00Z',
  updatedAt: '2026-08-09T00:00:00Z',
};

function renderWithApp(element: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{element}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Phase 4 shared profile experience', () => {
  beforeEach(() => {
    auth.loading = false;
    auth.session.actor = 'profile_picker';
    auth.session.childId = null;
    auth.session.parentMode = false;
    auth.enterChild.mockReset();
    auth.leaveChild.mockReset();
    vi.mocked(profileSummaryApi.get).mockReset();
    vi.mocked(profileSummaryApi.get).mockResolvedValue({
      date: '2026-08-09',
      timezone: 'Asia/Jakarta',
      pending: 0,
      children: [],
    });
    vi.mocked(profilesApi.list).mockReset();
    vi.mocked(childrenApi.list).mockReset();
    vi.mocked(childrenApi.create).mockReset();
    vi.mocked(childrenApi.update).mockReset();
    vi.mocked(childrenApi.archive).mockReset();
  });

  afterEach(cleanup);

  it('announces loading, then renders the limited profile projection as keyboard buttons', async () => {
    let resolveProfiles!: (value: (typeof profile)[]) => void;
    vi.mocked(profilesApi.list).mockReturnValue(
      new Promise((resolve) => {
        resolveProfiles = resolve;
      }),
    );
    renderWithApp(<ProfilePicker />);

    expect(screen.getByRole('status')).toHaveTextContent(
      'Loading family profiles',
    );
    resolveProfiles([profile]);

    const choice = await screen.findByRole('button', {
      name: 'Ari, PIN required',
    });
    choice.focus();
    expect(choice).toHaveFocus();
    expect(screen.queryByText(/created/i)).not.toBeInTheDocument();
  });

  it('shows an error and retries the profile projection', async () => {
    vi.mocked(profilesApi.list)
      .mockRejectedValueOnce(new Error('Profiles are unavailable.'))
      .mockResolvedValueOnce([]);
    renderWithApp(<ProfilePicker />);

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Profiles are unavailable.',
    );
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(
      await screen.findByRole('heading', { name: 'No child profiles yet' }),
    ).toBeInTheDocument();
    expect(profilesApi.list).toHaveBeenCalledTimes(2);
    expect(
      screen.getByRole('link', { name: /parent mode/i }),
    ).toBeInTheDocument();
  });

  it('requires and sanitizes a protected profile PIN before entering', async () => {
    vi.mocked(profilesApi.list).mockResolvedValue([profile]);
    auth.enterChild.mockResolvedValue({ actor: 'child' });
    renderWithApp(<ProfilePicker />);

    await userEvent.click(
      await screen.findByRole('button', { name: 'Ari, PIN required' }),
    );
    const pin = screen.getByLabelText('PIN');
    expect(pin).toHaveFocus();
    await userEvent.type(pin, '12a34567');
    expect(pin).toHaveValue('123456');
    await userEvent.click(
      screen.getByRole('button', { name: 'Enter profile' }),
    );
    expect(auth.enterChild).toHaveBeenCalledWith({
      childId: profile.id,
      pin: '123456',
    });
  });

  it('adds parent-only approved and waiting points to the post-login profile cards', async () => {
    auth.session.actor = 'parent';
    auth.session.parentMode = true;
    vi.mocked(profilesApi.list).mockResolvedValue([profile]);
    vi.mocked(profileSummaryApi.get).mockResolvedValue({
      date: '2026-08-09',
      timezone: 'Asia/Jakarta',
      pending: 1,
      children: [
        {
          childId: profile.id,
          nickname: profile.nickname,
          avatar: profile.avatar,
          color: profile.color,
          completed: 1,
          total: 2,
          pending: 1,
          approvedPointsToday: 12,
          waitingPointsToday: 5,
        },
      ],
    });

    renderWithApp(<ProfilePicker />);

    expect(
      await screen.findByRole('button', {
        name: 'Ari, 12 approved points and 5 waiting points today, PIN required',
      }),
    ).toBeInTheDocument();
    expect(screen.getByText('Approved today')).toBeInTheDocument();
    expect(screen.getByText('Waiting')).toBeInTheDocument();
  });

  it('shows today point summaries on the shared profile picker after sign-in', async () => {
    vi.mocked(profilesApi.list).mockResolvedValue([profile]);
    vi.mocked(profileSummaryApi.get).mockResolvedValue({
      date: '2026-08-09',
      timezone: 'Asia/Jakarta',
      pending: 0,
      children: [
        {
          childId: profile.id,
          nickname: profile.nickname,
          avatar: profile.avatar,
          color: profile.color,
          completed: 0,
          total: 1,
          pending: 0,
          approvedPointsToday: 7,
          waitingPointsToday: 0,
        },
      ],
    });
    renderWithApp(<ProfilePicker />);

    expect(
      await screen.findByRole('button', {
        name: 'Ari, 7 approved points and 0 waiting points today, PIN required',
      }),
    ).toBeInTheDocument();
    expect(profileSummaryApi.get).toHaveBeenCalledOnce();
    expect(screen.getByText('Approved today')).toBeInTheDocument();
  });

  it('identifies the exact active child and leaves through the server transition', async () => {
    auth.session.actor = 'child';
    auth.session.childId = profile.id;
    vi.mocked(profilesApi.list).mockResolvedValue([profile]);
    auth.leaveChild.mockResolvedValue({ actor: 'profile_picker' });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/child/today']}>
          <Routes>
            <Route path="/" element={<div>Picker returned</div>} />
            <Route path="/child" element={<ChildShell />}>
              <Route path="today" element={<div>Today content</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText('Ari')).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole('button', { name: 'Switch profile' }),
    );
    expect(auth.leaveChild).toHaveBeenCalledOnce();
    expect(await screen.findByText('Picker returned')).toBeInTheDocument();
  });

  it('keeps child and parent routes behind exact actor guards', async () => {
    auth.session.actor = 'child';
    auth.session.childId = profile.id;
    vi.mocked(profilesApi.list).mockResolvedValue([profile]);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/parent']}>
          <Routes>
            <Route path="/parent/unlock" element={<div>Parent unlock</div>} />
            <Route element={<RequireParent />}>
              <Route path="/parent" element={<div>Parent private</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(await screen.findByText('Parent unlock')).toBeInTheDocument();
    expect(screen.queryByText('Parent private')).not.toBeInTheDocument();
  });
});

describe('Phase 4 parent child management', () => {
  beforeEach(() => {
    vi.mocked(childrenApi.list).mockReset().mockResolvedValue([child]);
    vi.mocked(childrenApi.update).mockReset();
    vi.mocked(childrenApi.archive).mockReset();
  });

  afterEach(cleanup);

  it('validates the editor and preserves the PIN when a replacement is omitted', async () => {
    vi.mocked(childrenApi.update).mockResolvedValue(child);
    renderWithApp(<Children />);
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await userEvent.clear(screen.getByLabelText('Nickname'));
    await userEvent.click(screen.getByRole('button', { name: 'Save profile' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter a nickname.',
    );

    await userEvent.type(screen.getByLabelText('Nickname'), 'Ari Updated');
    await userEvent.click(screen.getByRole('button', { name: 'Save profile' }));
    await waitFor(() =>
      expect(childrenApi.update).toHaveBeenCalledWith(profile.id, {
        nickname: 'Ari Updated',
        avatar: 'fox',
        color: '#F5B94C',
      }),
    );
  });

  it('sends null only when a parent explicitly removes the current PIN', async () => {
    vi.mocked(childrenApi.update).mockResolvedValue({
      ...child,
      pinEnabled: false,
    });
    renderWithApp(<Children />);
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await userEvent.click(screen.getByLabelText('Remove the current PIN'));
    await userEvent.click(screen.getByRole('button', { name: 'Save profile' }));
    await waitFor(() =>
      expect(childrenApi.update).toHaveBeenCalledWith(profile.id, {
        nickname: 'Ari',
        avatar: 'fox',
        color: '#F5B94C',
        pin: null,
      }),
    );
  });

  it('requires an explicit accessible confirmation before archiving', async () => {
    vi.mocked(childrenApi.archive).mockResolvedValue(undefined);
    renderWithApp(<Children />);
    await userEvent.click(
      await screen.findByRole('button', { name: 'Archive' }),
    );

    const dialog = screen.getByRole('alertdialog', {
      name: 'Archive Ari?',
    });
    expect(dialog).toHaveTextContent('history will be preserved');
    expect(childrenApi.archive).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole('button', { name: 'Archive profile' }),
    );
    await waitFor(() =>
      expect(childrenApi.archive).toHaveBeenCalledWith(profile.id),
    );
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
    );
  });
});
