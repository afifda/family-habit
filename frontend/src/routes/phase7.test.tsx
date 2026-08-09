import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactNode } from 'react';
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { ChildPoints } from './ChildPoints';
import { Reports } from './Reports';
import { ReviewQueue } from './ReviewQueue';
import { ApiError } from '../api/errors';

const mocks = vi.hoisted(() => ({
  pending: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
  reverse: vi.fn(),
  balance: vi.fn(),
  ledger: vi.fn(),
  history: vi.fn(),
  correct: vi.fn(),
  children: vi.fn(),
  household: vi.fn(),
  report: vi.fn(),
}));

vi.mock('../api/client', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/client')>();
  return {
    ...original,
    reviewApi: {
      pending: mocks.pending,
      approve: mocks.approve,
      reject: mocks.reject,
      reverse: mocks.reverse,
    },
    pointsApi: {
      balance: mocks.balance,
      ledger: mocks.ledger,
      history: mocks.history,
      correct: mocks.correct,
    },
    reportsApi: { child: mocks.report },
    childrenApi: { list: mocks.children },
    householdApi: { get: mocks.household },
  };
});

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    session: { actor: 'child', childId: 'child-1', householdId: 'family-1' },
  }),
}));

const reviewItem = {
  id: 'completion-1',
  occurrenceId: 'occurrence-1',
  childId: 'child-1',
  childName: 'Maya',
  childAvatar: 'fox',
  childColor: '#326a4a',
  title: 'Read for 15 minutes',
  type: 'habit' as const,
  localDate: '2026-08-09',
  dueDate: null,
  points: 10,
  attemptNumber: 1,
  attemptStatus: 'pending' as const,
  occurrenceStatus: 'pending_approval' as const,
  occurrenceVersion: 2,
  submittedAt: '2026-08-09T06:42:00Z',
  availableActions: ['approve', 'reject'],
};

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

describe('Phase 7 points workflow', () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.pending.mockResolvedValue({
      data: [reviewItem],
      page: { nextCursor: null },
    });
    mocks.approve.mockResolvedValue({
      ...reviewItem,
      attemptStatus: 'approved',
      occurrenceStatus: 'approved',
      occurrenceVersion: 3,
    });
    mocks.reject.mockResolvedValue({
      ...reviewItem,
      attemptStatus: 'rejected',
      occurrenceStatus: 'not_started',
      occurrenceVersion: 3,
    });
    mocks.balance.mockResolvedValue({
      childId: 'child-1',
      points: 35,
      asOf: '2026-08-09T07:00:00Z',
    });
    mocks.ledger.mockResolvedValue({
      data: [
        {
          id: 'ledger-1',
          childId: 'child-1',
          kind: 'award',
          amount: 10,
          reason: '',
          occurrenceId: 'occurrence-1',
          title: 'Read',
          createdAt: '2026-08-09T07:00:00Z',
        },
      ],
      page: { nextCursor: null },
    });
    mocks.children.mockResolvedValue([
      {
        id: 'child-1',
        nickname: 'Maya',
        avatar: 'fox',
        color: '#326a4a',
        active: true,
      },
    ]);
    mocks.household.mockResolvedValue({
      timezone: 'Europe/Berlin',
      weekStartsOn: 'monday',
    });
    mocks.report.mockResolvedValue({
      childId: 'child-1',
      period: 'week',
      startDate: '2026-08-03',
      endDate: '2026-08-09',
      timezone: 'Europe/Berlin',
      weekStartsOn: 1,
      assigned: 6,
      submitted: 5,
      pending: 1,
      approved: 3,
      reversed: 0,
      rejected: 1,
      incomplete: 2,
      cancelled: 0,
      pointsEarned: 25,
      manualCorrections: 5,
      netPointsChange: 30,
    });
    mocks.history.mockResolvedValue({
      data: [
        {
          id: 'occurrence-1',
          childId: 'child-1',
          type: 'habit',
          localDate: '2026-08-09',
          title: 'Read',
          points: 10,
          status: 'approved',
          version: 3,
          awardDelta: 10,
          reversalDelta: 0,
          attempts: [
            {
              id: 'completion-1',
              attemptNumber: 1,
              status: 'approved',
              submittedAt: '2026-08-09T06:42:00Z',
              decidedAt: '2026-08-09T07:00:00Z',
            },
          ],
        },
      ],
      page: { nextCursor: null },
    });
  });

  it('prevents duplicate approval taps and announces the exact award', async () => {
    const user = userEvent.setup();
    let resolve!: (value: unknown) => void;
    mocks.approve.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    renderPage(<ReviewQueue />);
    const approve = await screen.findByRole('button', {
      name: 'Approve · +10',
    });
    await user.dblClick(approve);
    expect(mocks.approve).toHaveBeenCalledTimes(1);
    expect(approve).toBeDisabled();
    resolve({ ...reviewItem, occurrenceVersion: 3 });
    expect(await screen.findByRole('status')).toHaveTextContent(
      '10 points added to Maya',
    );
  });

  it('sends back work with optional child-safe guidance and no punishment copy', async () => {
    const user = userEvent.setup();
    renderPage(<ReviewQueue />);
    await user.click(await screen.findByRole('button', { name: 'Not yet' }));
    const dialog = screen.getByRole('dialog', {
      name: 'Ready for another try?',
    });
    expect(dialog).toHaveTextContent('No points will be removed');
    await user.type(
      within(dialog).getByLabelText('Kind note (optional)'),
      'Please finish the last page.',
    );
    await user.click(
      within(dialog).getByRole('button', { name: 'Return to To do' }),
    );
    expect(mocks.reject).toHaveBeenCalledWith(
      'completion-1',
      2,
      'Please finish the last page.',
      expect.any(String),
    );
  });

  it('shows only the active child balance and neutral reversal activity language', async () => {
    mocks.ledger.mockResolvedValue({
      data: [
        {
          id: 'ledger-2',
          childId: 'child-1',
          kind: 'approval_reversal',
          amount: -10,
          reason: 'Parent record',
          createdAt: '2026-08-09T07:00:00Z',
        },
      ],
      page: { nextCursor: null },
    });
    renderPage(<ChildPoints />);
    expect(
      await screen.findByRole('heading', { name: '35' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Points updated by a parent')).toBeInTheDocument();
    expect(screen.queryByText('Parent record')).not.toBeInTheDocument();
  });

  it('renders explicit report boundaries and traceable 30-day history', async () => {
    renderPage(<Reports />);
    expect(
      await screen.findByRole('heading', { name: 'Maya’s report' }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText('Aug 3, 2026 – Aug 9, 2026'),
    ).toBeInTheDocument();
    expect(screen.getByText('Recent history')).toBeInTheDocument();
    expect(screen.getByText('Read', { exact: true })).toBeInTheDocument();
  });

  it('exposes recoverable loading errors', async () => {
    mocks.pending.mockRejectedValue(new TypeError('offline'));
    const user = userEvent.setup();
    renderPage(<ReviewQueue />);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'could not load the review queue',
    );
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(mocks.pending).toHaveBeenCalledTimes(2));
  });

  it('loads every pending page and keeps household child filters available', async () => {
    const second = {
      ...reviewItem,
      id: 'completion-2',
      childId: 'child-2',
      childName: 'Leo',
      title: 'Make the bed',
    };
    mocks.children.mockResolvedValue([
      {
        id: 'child-1',
        nickname: 'Maya',
        avatar: 'fox',
        color: '#326a4a',
        active: true,
      },
      {
        id: 'child-2',
        nickname: 'Leo',
        avatar: 'panda',
        color: '#245039',
        active: false,
      },
    ]);
    mocks.pending.mockImplementation(
      (_child: string | undefined, cursor: string | undefined) =>
        Promise.resolve(
          cursor
            ? { data: [second], page: { nextCursor: null } }
            : { data: [reviewItem], page: { nextCursor: 'page-2' } },
        ),
    );
    const user = userEvent.setup();
    renderPage(<ReviewQueue />);
    expect(
      await screen.findByRole('button', { name: 'Leo' }),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole('button', { name: 'Load more submissions' }),
    );
    expect(await screen.findByText('Make the bed')).toBeInTheDocument();
    expect(mocks.pending).toHaveBeenLastCalledWith(undefined, 'page-2');
  });

  it('replays an ambiguous approval with the same key and refreshes stale conflicts', async () => {
    const user = userEvent.setup();
    mocks.approve
      .mockRejectedValueOnce(new TypeError('Connection lost'))
      .mockResolvedValueOnce({ ...reviewItem, occurrenceVersion: 3 });
    renderPage(<ReviewQueue />);
    const approve = await screen.findByRole('button', {
      name: 'Approve · +10',
    });
    await user.click(approve);
    await screen.findByRole('alert');
    await user.click(approve);
    await waitFor(() => expect(mocks.approve).toHaveBeenCalledTimes(2));
    expect(mocks.approve.mock.calls[0]?.[2]).toBe(
      mocks.approve.mock.calls[1]?.[2],
    );

    mocks.approve.mockRejectedValueOnce(
      new ApiError(409, {
        error: { code: 'version_conflict', message: 'Changed' },
      }),
    );
    await user.click(approve);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'queue was refreshed',
    );
    await waitFor(() =>
      expect(mocks.pending.mock.calls.length).toBeGreaterThan(1),
    );
  });

  it('traps rejection dialog focus, closes on Escape, and restores the trigger', async () => {
    const user = userEvent.setup();
    renderPage(<ReviewQueue />);
    const trigger = await screen.findByRole('button', { name: 'Not yet' });
    await user.click(trigger);
    const dialog = screen.getByRole('dialog');
    const note = within(dialog).getByLabelText('Kind note (optional)');
    expect(note).toHaveFocus();
    await user.keyboard('{Shift>}{Tab}{/Shift}');
    expect(
      within(dialog).getByRole('button', { name: 'Return to To do' }),
    ).toHaveFocus();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('confirms the exact signed reversal and traces correction ledger effects', async () => {
    mocks.reverse.mockResolvedValue({ occurrenceVersion: 4 });
    mocks.ledger.mockResolvedValue({
      data: [
        {
          id: 'bonus-1',
          childId: 'child-1',
          kind: 'manual_correction',
          amount: 5,
          reason: 'Birthday bonus',
          createdAt: '2026-08-09T07:00:00Z',
        },
      ],
      page: { nextCursor: null },
    });
    const user = userEvent.setup();
    renderPage(<Reports />);
    await user.click(
      await screen.findByRole('button', { name: 'Reverse approval' }),
    );
    const dialog = screen.getByRole('dialog', {
      name: 'Reverse this approval?',
    });
    expect(dialog).toHaveTextContent('changes the balance by -10 points');
    expect(await screen.findByText('Birthday bonus')).toBeInTheDocument();
  });

  it('uses a new correction key after the editable payload changes', async () => {
    mocks.correct
      .mockRejectedValueOnce(new TypeError('Connection lost'))
      .mockResolvedValueOnce({ id: 'bonus-2' });
    const user = userEvent.setup();
    renderPage(<Reports />);
    await user.click(
      await screen.findByRole('button', { name: 'Add bonus points' }),
    );
    const dialog = screen.getByRole('dialog', { name: 'Add bonus points' });
    await user.type(within(dialog).getByLabelText('Points'), '5');
    const reason = within(dialog).getByLabelText('Reason');
    await user.type(reason, 'Helping with dinner');
    await user.click(
      within(dialog).getByRole('button', { name: 'Review bonus' }),
    );
    const confirmation = screen.getByRole('dialog', {
      name: 'Confirm bonus points',
    });
    expect(confirmation).toHaveTextContent(
      'change Maya’s balance by +5 points',
    );
    await user.click(
      within(confirmation).getByRole('button', {
        name: 'Confirm +5 points',
      }),
    );
    await screen.findByRole('alert');
    const firstKey = String(mocks.correct.mock.calls[0]?.[3]);
    await user.click(
      within(confirmation).getByRole('button', { name: 'Go back' }),
    );
    const reopened = screen.getByRole('dialog', { name: 'Add bonus points' });
    await user.type(within(reopened).getByLabelText('Reason'), ' today');
    await user.click(
      within(reopened).getByRole('button', { name: 'Review bonus' }),
    );
    await user.click(
      within(
        screen.getByRole('dialog', { name: 'Confirm bonus points' }),
      ).getByRole('button', { name: 'Confirm +5 points' }),
    );
    await waitFor(() => expect(mocks.correct).toHaveBeenCalledTimes(2));
    expect(mocks.correct.mock.calls[1]?.[3]).not.toBe(firstKey);
  });

  it('renders locked or permission loss as a recoverable parent-only error', async () => {
    mocks.pending.mockRejectedValue(
      new ApiError(403, {
        error: { code: 'forbidden', message: 'Parent Mode is locked.' },
      }),
    );
    renderPage(<ReviewQueue />);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'could not load the review queue',
    );
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument();
  });
});
