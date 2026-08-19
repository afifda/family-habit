import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
/* eslint-disable @typescript-eslint/unbound-method */
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  childrenApi,
  householdApi,
  rewardsApi,
  routineGroupsApi,
} from '../api/client';
import { ChildRewards } from './ChildRewards';
import { ParentRewards } from './ParentRewards';
import { RoutineGroups } from './RoutineGroups';

vi.mock('../api/client', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/client')>();
  return {
    ...original,
    routineGroupsApi: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      archive: vi.fn(),
      reorder: vi.fn(),
    },
    childrenApi: {
      list: vi.fn(),
    },
    householdApi: {
      get: vi.fn(),
    },
    rewardsApi: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      archive: vi.fn(),
      redemptions: vi.fn(),
      fulfill: vi.fn(),
      cancel: vi.fn(),
      childCatalog: vi.fn(),
      childRedemptions: vi.fn(),
      redeem: vi.fn(),
    },
  };
});

function renderPage(page: React.ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      {page}
    </QueryClientProvider>,
  );
}

describe('Phase 9 routines and rewards', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(routineGroupsApi.list).mockResolvedValue([
      {
        id: 'morning',
        name: 'Morning',
        icon: '☀',
        color: '#f2b84b',
        startsAtLocal: '07:00',
        endsAtLocal: '09:00',
        sortOrder: 0,
        version: 1,
        archivedAt: null,
      },
      {
        id: 'evening',
        name: 'Evening',
        icon: '☾',
        color: '#8090c0',
        sortOrder: 1,
        version: 1,
        archivedAt: null,
      },
    ]);
    vi.mocked(routineGroupsApi.reorder).mockResolvedValue(undefined);
    vi.mocked(rewardsApi.childCatalog).mockResolvedValue({
      balance: 40,
      data: [
        {
          id: 'reward-1',
          title: 'Choose movie night',
          description: 'Pick our next family film',
          icon: '🎬',
          costPoints: 25,
          canRedeem: true,
          version: 3,
        },
      ],
    });
    vi.mocked(rewardsApi.childRedemptions).mockResolvedValue([]);
    vi.mocked(householdApi.get).mockResolvedValue({
      id: 'household-1',
      name: 'Test household',
      timezone: 'Asia/Jakarta',
      weekStartsOn: 'sunday',
      parentModeTimeoutMinutes: 15,
      rewardsEnabled: true,
      version: 1,
    });
    vi.mocked(childrenApi.list).mockResolvedValue([
      {
        id: 'child-1',
        nickname: 'Ari',
        avatar: 'fox',
        color: '#f2b84b',
        pinEnabled: false,
        active: true,
        createdAt: '2026-08-18T00:00:00Z',
        updatedAt: '2026-08-18T00:00:00Z',
      },
    ]);
    vi.mocked(rewardsApi.list).mockResolvedValue([]);
    vi.mocked(rewardsApi.redemptions).mockResolvedValue([]);
    vi.mocked(rewardsApi.archive).mockResolvedValue(undefined);
    vi.mocked(rewardsApi.fulfill).mockResolvedValue({
      id: 'redemption-1',
      childId: 'child-1',
      rewardId: 'reward-1',
      rewardTitle: 'Choose movie night',
      costPoints: 25,
      state: 'fulfilled',
      requestedAt: '2026-08-18T00:00:00Z',
      version: 2,
    });
    vi.mocked(rewardsApi.cancel).mockResolvedValue({
      id: 'redemption-1',
      childId: 'child-1',
      rewardId: 'reward-1',
      rewardTitle: 'Choose movie night',
      costPoints: 25,
      state: 'cancelled',
      requestedAt: '2026-08-18T00:00:00Z',
      version: 2,
    });
  });

  afterEach(cleanup);

  it('offers keyboard-operable routine ordering with full-list persistence', async () => {
    const user = userEvent.setup();
    renderPage(<RoutineGroups />);
    await user.click(
      await screen.findByRole('button', { name: 'Move Evening up' }),
    );
    expect(routineGroupsApi.reorder).toHaveBeenCalledWith([
      expect.objectContaining({ id: 'evening' }),
      expect.objectContaining({ id: 'morning' }),
    ]);
  });

  it('lets parents choose suggested routine icons or enter a custom icon', async () => {
    const user = userEvent.setup();
    vi.mocked(routineGroupsApi.create).mockResolvedValue({
      id: 'after-school',
      name: 'After school',
      icon: '☀',
      color: '#f9ee71',
      startsAtLocal: '05:00',
      endsAtLocal: '11:59',
      sortOrder: 2,
      version: 1,
      archivedAt: null,
    });
    renderPage(<RoutineGroups />);

    await user.click(await screen.findByRole('button', { name: 'New group' }));
    await user.type(screen.getByLabelText('Name'), 'Morning Routine');
    await user.click(screen.getByRole('button', { name: 'Use 🪥 icon' }));
    expect(screen.getByRole('button', { name: 'Use 🪥 icon' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await user.clear(screen.getByLabelText('Custom icon'));
    await user.type(screen.getByLabelText('Custom icon'), '☀');
    await user.click(screen.getByRole('button', { name: 'Save group' }));

    await waitFor(() =>
      expect(routineGroupsApi.create).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'Morning Routine',
          icon: '☀',
        }),
        expect.any(String),
      ),
    );
  });

  it('lets parents choose suggested reward icons or enter a custom icon', async () => {
    const user = userEvent.setup();
    vi.mocked(rewardsApi.create).mockResolvedValue({
      id: 'reward-2',
      title: 'Puzzle time',
      description: '',
      icon: 'VIP',
      costPoints: 25,
      availabilityScope: 'all_active_children',
      eligibleChildIds: [],
      active: true,
      version: 1,
    });
    renderPage(<ParentRewards />);

    await user.click(await screen.findByRole('button', { name: 'New reward' }));
    await user.type(screen.getByLabelText('Title'), 'Puzzle time');
    await user.click(screen.getByRole('button', { name: 'Use 🧩 icon' }));
    expect(screen.getByRole('button', { name: 'Use 🧩 icon' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await user.clear(screen.getByLabelText('Custom icon'));
    await user.type(screen.getByLabelText('Custom icon'), 'VIP');
    await user.click(screen.getByRole('button', { name: 'Save reward' }));

    await waitFor(() =>
      expect(rewardsApi.create).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Puzzle time',
          icon: 'VIP',
        }),
        expect.any(String),
      ),
    );
  });

  it('shows exact signed redemption arithmetic and prevents duplicate activation', async () => {
    const user = userEvent.setup();
    let resolve!: (
      value: Awaited<ReturnType<typeof rewardsApi.redeem>>,
    ) => void;
    vi.mocked(rewardsApi.redeem).mockImplementation(
      () => new Promise((done) => (resolve = done)),
    );
    renderPage(<ChildRewards />);
    await user.click(
      await screen.findByRole('button', { name: 'Choose reward' }),
    );
    expect(screen.getByText('−25')).toBeInTheDocument();
    expect(screen.getByText('15')).toBeInTheDocument();
    const confirm = screen.getByRole('button', { name: 'Confirm −25 points' });
    await user.click(confirm);
    expect(confirm).toBeDisabled();
    expect(rewardsApi.redeem).toHaveBeenCalledWith(
      'reward-1',
      3,
      25,
      expect.any(String),
    );
    expect(rewardsApi.redeem).toHaveBeenCalledTimes(1);
    resolve({
      id: 'redemption-1',
      childId: 'child-1',
      rewardId: 'reward-1',
      rewardTitle: 'Choose movie night',
      costPoints: 25,
      state: 'requested',
      requestedAt: '2026-08-18T08:00:00Z',
      version: 1,
    });
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
  });
});
