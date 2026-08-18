import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactNode } from 'react';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { HouseholdSettings } from './HouseholdSettings';
import { ParentOverview } from './ParentOverview';

const mocks = vi.hoisted(() => ({
  children: vi.fn(),
  household: vi.fn(),
  updateHousehold: vi.fn(),
  overview: vi.fn(),
  balance: vi.fn(),
  report: vi.fn(),
}));

vi.mock('../api/client', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/client')>();
  return {
    ...original,
    childrenApi: { list: mocks.children },
    householdApi: {
      get: mocks.household,
      update: mocks.updateHousehold,
    },
    overviewApi: { get: mocks.overview },
    pointsApi: { balance: mocks.balance },
    reportsApi: { child: mocks.report },
  };
});

function renderPage(page: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{page}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const household = {
  id: 'household-1',
  name: 'River Family',
  timezone: 'Asia/Jakarta',
  weekStartsOn: 'monday' as const,
  parentModeTimeoutMinutes: 15 as const,
  version: 1,
};

describe('Phase 8 integrated frontend quality', () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.household.mockResolvedValue(household);
    mocks.overview.mockResolvedValue({
      date: '2026-08-09',
      timezone: 'Asia/Jakarta',
      pending: 1,
      children: [
        {
          childId: 'child-1',
          nickname: 'Maya with a wonderfully long nickname',
          avatar: 'fox',
          color: '#326a4a',
          completed: 3,
          total: 4,
          pending: 1,
          approvedPointsToday: 24,
          waitingPointsToday: 8,
        },
      ],
    });
    mocks.children.mockResolvedValue([
      {
        id: 'child-1',
        nickname: 'Maya with a wonderfully long nickname',
        avatar: 'fox',
        color: '#326a4a',
        active: true,
      },
    ]);
    mocks.balance.mockResolvedValue({ childId: 'child-1', points: 35 });
    mocks.updateHousehold.mockResolvedValue(household);
  });

  it('shows household-local completed/total and per-child pending data', async () => {
    renderPage(<ParentOverview />);

    expect(
      await screen.findByRole('heading', { name: 'Family overview' }),
    ).toBeInTheDocument();
    expect(screen.getByText('3 of 4 completed')).toBeInTheDocument();
    expect(
      screen.getByRole('progressbar', {
        name: "Maya with a wonderfully long nickname's completed work today",
      }),
    ).toHaveAttribute('value', '3');
    expect(screen.getAllByText('1 waiting', { exact: false })).toHaveLength(2);
    expect(
      screen.getByRole('link', { name: /1 waiting for review/i }),
    ).toHaveAttribute('href', '/parent/review');
    expect(mocks.overview).toHaveBeenCalledTimes(1);
  });

  it('keeps settings input after a server failure and supports safe retry', async () => {
    const user = userEvent.setup();
    mocks.updateHousehold
      .mockRejectedValueOnce(new Error('The server is unavailable.'))
      .mockResolvedValueOnce({ ...household, name: 'New family name' });
    renderPage(<HouseholdSettings />);

    const name = await screen.findByLabelText('Household name');
    await user.clear(name);
    await user.type(name, 'New family name');
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Your changes are still here',
    );
    expect(name).toHaveValue('New family name');

    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Household settings saved',
    );
    expect(mocks.updateHousehold).toHaveBeenLastCalledWith(
      expect.objectContaining({ name: 'New family name' }),
    );
  });

  it('retains all primary overview content at phone, tablet, and desktop widths', async () => {
    for (const [width, height] of [
      [320, 568],
      [768, 1024],
      [1440, 900],
    ]) {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: width,
      });
      Object.defineProperty(window, 'innerHeight', {
        configurable: true,
        value: height,
      });
      window.dispatchEvent(new Event('resize'));
      const view = renderPage(<ParentOverview />);
      expect(await screen.findByText('3 of 4 completed')).toBeVisible();
      expect(
        screen.getByRole('link', { name: /View progress/i }),
      ).toBeVisible();
      view.unmount();
      await waitFor(() =>
        expect(screen.queryByText('3 of 4 completed')).not.toBeInTheDocument(),
      );
    }
  });
});
