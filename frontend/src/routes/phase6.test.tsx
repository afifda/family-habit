import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../api/errors';
import { ChildToday } from './ChildToday';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  submit: vi.fn(),
  withdraw: vi.fn(),
}));

vi.mock('../api/client', async (importOriginal) => {
  const original = await importOriginal<typeof import('../api/client')>();
  return { ...original, todayApi: mocks };
});

vi.mock('../auth/AuthProvider', () => ({
  useAuth: () => ({
    session: {
      actor: 'child',
      childId: 'child-1',
      householdId: 'household-1',
    },
  }),
}));

const today = {
  childId: 'child-1',
  date: '2026-08-09',
  timezone: 'Europe/Berlin',
  occurrences: [
    {
      id: 'todo-1',
      childId: 'child-1',
      type: 'habit' as const,
      localDate: '2026-08-09',
      title: 'Brush teeth',
      description: 'Brush for two minutes.',
      points: 5,
      version: 1,
      status: 'not_started' as const,
      group: 'to_do' as const,
      dueState: 'scheduled_today' as const,
      availableActions: ['submit' as const],
    },
    {
      id: 'late-1',
      childId: 'child-1',
      type: 'task' as const,
      localDate: '2026-08-08',
      dueDate: '2026-08-08',
      title: 'Put clothes away',
      points: 8,
      version: 1,
      status: 'not_started' as const,
      group: 'to_do' as const,
      dueState: 'overdue' as const,
      availableActions: ['submit' as const],
    },
    {
      id: 'wait-1',
      childId: 'child-1',
      type: 'habit' as const,
      localDate: '2026-08-09',
      title: 'Read',
      points: 10,
      version: 2,
      status: 'pending_approval' as const,
      group: 'waiting_for_parent' as const,
      dueState: 'scheduled_today' as const,
      completionId: 'completion-1',
      availableActions: ['withdraw' as const],
    },
    {
      id: 'done-1',
      childId: 'child-1',
      type: 'habit' as const,
      localDate: '2026-08-09',
      title: 'Make the bed',
      points: 5,
      version: 2,
      status: 'approved' as const,
      group: 'done' as const,
      dueState: 'scheduled_today' as const,
      availableActions: [],
    },
  ],
};

function renderToday() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ChildToday />
    </QueryClientProvider>,
  );
}

describe('Child Today', () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockResolvedValue(structuredClone(today));
    mocks.submit.mockResolvedValue({
      id: 'completion-new',
      occurrenceId: 'todo-1',
      childId: 'child-1',
      attemptStatus: 'pending',
      submittedAt: '2026-08-09T08:00:00Z',
      occurrenceStatus: 'pending_approval',
      occurrenceVersion: 2,
    });
    mocks.withdraw.mockResolvedValue({
      id: 'completion-1',
      occurrenceId: 'wait-1',
      childId: 'child-1',
      attemptStatus: 'withdrawn',
      submittedAt: '2026-08-09T07:00:00Z',
      occurrenceStatus: 'not_started',
      occurrenceVersion: 3,
    });
  });

  it('groups activities with child-friendly type, points, and neutral due labels', async () => {
    renderToday();
    expect(
      await screen.findByRole('heading', { name: 'Other · 4' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('radio', { name: 'Waiting · 1' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Done · 1' })).toBeInTheDocument();
    expect(
      screen.getByText(/One-off task · Still to do · Due Aug 8 · 8 points/),
    ).toBeInTheDocument();
    expect(screen.getByText('1 of 4', { exact: false })).toBeInTheDocument();
    expect(mocks.get).toHaveBeenCalledWith('child-1');
  });

  it('opens detail and submits once despite rapid taps', async () => {
    const user = userEvent.setup();
    let resolveSubmit!: (value: unknown) => void;
    mocks.submit.mockReturnValue(
      new Promise((resolve) => {
        resolveSubmit = resolve;
      }),
    );
    renderToday();
    await user.click(
      await screen.findByRole('button', {
        name: 'View details for Brush teeth',
      }),
    );
    expect(screen.getByRole('dialog')).toHaveTextContent(
      'Brush for two minutes.',
    );
    const action = within(screen.getByRole('dialog')).getByRole('button', {
      name: 'I did it',
    });
    await user.dblClick(action);
    expect(mocks.submit).toHaveBeenCalledTimes(1);
    expect(action).toBeDisabled();
    resolveSubmit({
      id: 'completion-new',
      occurrenceId: 'todo-1',
      childId: 'child-1',
      attemptStatus: 'pending',
      submittedAt: '2026-08-09T08:00:00Z',
      occurrenceStatus: 'pending_approval',
      occurrenceVersion: 2,
    });
    await waitFor(() =>
      expect(screen.getByRole('dialog')).toHaveTextContent(
        'Waiting for a parent',
      ),
    );
  });

  it('contains keyboard focus, closes with Escape, and restores its trigger', async () => {
    const user = userEvent.setup();
    renderToday();
    const trigger = await screen.findByRole('button', {
      name: 'View details for Brush teeth',
    });
    await user.click(trigger);

    const dialog = screen.getByRole('dialog');
    const close = within(dialog).getByRole('button', { name: /Today/ });
    const submit = within(dialog).getByRole('button', { name: 'I did it' });
    expect(close).toHaveFocus();

    await user.keyboard('{Shift>}{Tab}{/Shift}');
    expect(submit).toHaveFocus();
    await user.keyboard('{Tab}');
    expect(close).toHaveFocus();
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('focuses the destination group when a transition removed the detail trigger', async () => {
    const user = userEvent.setup();
    renderToday();
    await user.click(
      await screen.findByRole('button', {
        name: 'View details for Brush teeth',
      }),
    );
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'I did it',
      }),
    );
    await waitFor(() =>
      expect(screen.getByRole('dialog')).toHaveTextContent(
        'Waiting for a parent',
      ),
    );

    await user.keyboard('{Escape}');

    const destination = screen.getByRole('heading', {
      name: /Other/,
    });
    await waitFor(() => expect(destination).toHaveFocus());
  });

  it('withdraws a pending item and returns it to To do', async () => {
    const user = userEvent.setup();
    mocks.get.mockResolvedValueOnce(structuredClone(today)).mockResolvedValue({
      ...structuredClone(today),
      occurrences: today.occurrences.map((item) =>
        item.id === 'wait-1'
          ? {
              ...item,
              status: 'not_started' as const,
              group: 'to_do' as const,
              completionId: null,
              version: 3,
              availableActions: ['submit' as const],
            }
          : item,
      ),
    });
    renderToday();
    const undo = await screen.findByRole('button', {
      name: 'Withdraw submission',
    });
    await user.click(undo);
    await waitFor(() =>
      expect(mocks.withdraw).toHaveBeenCalledWith(
        'completion-1',
        2,
        expect.any(String),
      ),
    );
    expect(
      await screen.findByRole('radio', { name: 'To do · 3' }),
    ).toBeInTheDocument();
  });

  it('restores an item and offers retry feedback when submission fails', async () => {
    const user = userEvent.setup();
    mocks.submit.mockRejectedValueOnce(new Error('You appear to be offline.'));
    renderToday();
    const actions = await screen.findAllByRole('button', { name: 'I did it' });
    await user.click(actions[0]!);
    expect(await screen.findByRole('alert')).toHaveTextContent('Try again');
    expect(
      screen.getByRole('radio', { name: 'To do · 2' }),
    ).toBeInTheDocument();
    expect(mocks.get).toHaveBeenCalledTimes(1);
    const firstKey = mocks.submit.mock.calls[0]?.[2] as string;
    await user.click(screen.getAllByRole('button', { name: 'I did it' })[0]!);
    await waitFor(() => expect(mocks.submit).toHaveBeenCalledTimes(2));
    expect(mocks.submit.mock.calls[1]?.[2]).toBe(firstKey);
  });

  it('renders a retry state and a separate permission state', async () => {
    const user = userEvent.setup();
    mocks.get.mockRejectedValueOnce(new Error('Network unavailable'));
    renderToday();
    await user.click(await screen.findByRole('button', { name: 'Try again' }));
    expect(
      await screen.findByRole('heading', { name: 'Today' }),
    ).toBeInTheDocument();
  });

  it('does not offer retry when access is forbidden', async () => {
    mocks.get.mockRejectedValue(
      new ApiError(403, { error: { code: 'forbidden', message: 'Forbidden' } }),
    );
    renderToday();
    expect(
      await screen.findByRole('heading', {
        name: 'This is not your Today page',
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Try again' }),
    ).not.toBeInTheDocument();
  });

  it('shows an encouraging empty state', async () => {
    mocks.get.mockResolvedValue({ ...today, occurrences: [] });
    renderToday();
    expect(
      await screen.findByRole('heading', { name: 'You’re all caught up' }),
    ).toBeInTheDocument();
  });
});
