import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
/* eslint-disable @typescript-eslint/unbound-method */
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { rewardEligibilityApi, rewardsApi } from '../api/client';
import { RewardEligibilityPanel } from '../components/RewardEligibilityPanel';
import { ChildRewards } from './ChildRewards';

vi.mock('../api/client', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/client')>();
  return {
    ...original,
    rewardEligibilityApi: {
      policy: vi.fn(),
      updatePolicy: vi.fn(),
      progress: vi.fn(),
      evaluations: vi.fn(),
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

describe('Phase 9.1 periodic reward eligibility', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(rewardEligibilityApi.policy).mockResolvedValue({
      enabled: true,
      period: 'weekly',
      minimumPoints: 100,
      minimumCompletionPercentage: 80,
      maximumRedemptions: 1,
      graceHours: 24,
      effectiveFrom: '2026-08-24',
      version: 4,
    });
    vi.mocked(rewardEligibilityApi.progress).mockResolvedValue([
      {
        childId: 'child-1',
        childName: 'Sam',
        policyEnabled: true,
        collectionPeriodStart: '2026-08-17',
        collectionPeriodEnd: '2026-08-23',
        evaluationAt: '2026-08-24T23:59:59Z',
        pointsCollected: 75,
        completionPercentage: 85,
        assignedCount: 10,
        approvedCount: 8,
        status: 'collecting',
        eligibleFrom: null,
        eligibleUntil: null,
        redemptionsUsed: 0,
        maximumRedemptions: 1,
        rules: [
          { type: 'minimum_points', target: 100, actual: 75, passed: false },
          {
            type: 'minimum_completion_percentage',
            target: 80,
            actual: 85,
            passed: true,
          },
        ],
      },
    ]);
    vi.mocked(rewardEligibilityApi.evaluations).mockResolvedValue([]);
    vi.mocked(rewardEligibilityApi.updatePolicy).mockImplementation(
      (input, version) =>
        Promise.resolve({
          ...input,
          effectiveFrom: '2026-08-24',
          version: version + 1,
        }),
    );
    vi.mocked(rewardsApi.childRedemptions).mockResolvedValue([]);
  });

  afterEach(cleanup);

  it('saves versioned policy settings and explains next-period effect', async () => {
    const user = userEvent.setup();
    renderPage(<RewardEligibilityPanel />);

    expect(await screen.findByText('Sam')).toBeInTheDocument();
    expect(
      screen.getByText(/Changes take effect from the next weekly/),
    ).toBeInTheDocument();
    await user.clear(screen.getByLabelText('Minimum approved points'));
    await user.type(screen.getByLabelText('Minimum approved points'), '125');
    await user.click(
      screen.getByRole('button', { name: 'Save collection rules' }),
    );

    await waitFor(() =>
      expect(rewardEligibilityApi.updatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: true,
          period: 'weekly',
          minimumPoints: 125,
          minimumCompletionPercentage: 80,
          maximumRedemptions: 1,
          graceHours: 24,
        }),
        4,
        expect.any(String),
      ),
    );
  });

  it('renders server-computed collecting state without enabling redemption', async () => {
    vi.mocked(rewardsApi.childCatalog).mockResolvedValue({
      balance: 250,
      eligibility: {
        policyEnabled: true,
        status: 'collecting',
        collectionPeriodStart: '2026-08-17',
        collectionPeriodEnd: '2026-08-23',
        pointsCollected: 75,
        minimumPoints: 100,
        completionPercentage: 85,
        minimumCompletionPercentage: 80,
        eligibleFrom: null,
        eligibleUntil: null,
        redemptionsUsed: 0,
        maximumRedemptions: 1,
        canRedeem: false,
        unavailableReason: 'collection_in_progress',
        pointsShortfall: 25,
      },
      data: [
        {
          id: 'reward-1',
          title: 'Movie night',
          costPoints: 25,
          version: 2,
          canRedeem: false,
          shortfallPoints: 0,
        },
      ],
    });

    renderPage(<ChildRewards />);
    expect(await screen.findByText('Keep collecting')).toBeInTheDocument();
    expect(screen.getByText(/25 more approved points/)).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Choose reward' }),
    ).toBeDisabled();
    expect(rewardsApi.redeem).not.toHaveBeenCalled();
  });

  it('forces daily collection periods to use no approval grace', async () => {
    const user = userEvent.setup();
    renderPage(<RewardEligibilityPanel />);

    const period = await screen.findByLabelText('Collection period');
    await user.selectOptions(period, 'daily');
    const grace = screen.getByRole('combobox', {
      name: /Time for final approvals/,
    });
    expect(grace).toBeDisabled();
    expect(grace).toHaveValue('0');
    expect(
      screen.getByText(
        'Daily periods are evaluated without extra approval time.',
      ),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Save collection rules' }),
    );
    await waitFor(() =>
      expect(rewardEligibilityApi.updatePolicy).toHaveBeenCalledWith(
        expect.objectContaining({ period: 'daily', graceHours: 0 }),
        4,
        expect.any(String),
      ),
    );
  });
});
