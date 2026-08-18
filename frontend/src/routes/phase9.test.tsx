import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
/* eslint-disable @typescript-eslint/unbound-method */
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { rewardsApi, routineGroupsApi } from '../api/client';
import { ChildRewards } from './ChildRewards';
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
    rewardsApi: {
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
