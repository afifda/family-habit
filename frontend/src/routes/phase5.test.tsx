/* eslint-disable @typescript-eslint/unbound-method -- mocked API methods are inspected directly. */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  childrenApi,
  habitsApi,
  householdApi,
  routineGroupsApi,
  tasksApi,
  type Child,
  type Habit,
  type OneOffTask,
} from '../api/client';
import { HabitsTasks } from './HabitsTasks';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    childrenApi: { ...actual.childrenApi, list: vi.fn() },
    householdApi: { get: vi.fn() },
    habitsApi: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      deactivate: vi.fn(),
      assign: vi.fn(),
      updateAssignment: vi.fn(),
    },
    tasksApi: {
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      cancel: vi.fn(),
    },
    routineGroupsApi: { list: vi.fn() },
  };
});

const children: Child[] = [
  {
    id: 'child-1',
    nickname: 'Ari',
    avatar: 'fox',
    color: '#F5B94C',
    pinEnabled: false,
    active: true,
    createdAt: '2026-08-09T00:00:00Z',
    updatedAt: '2026-08-09T00:00:00Z',
  },
  {
    id: 'child-2',
    nickname: 'Bea',
    avatar: 'owl',
    color: '#B8D8BA',
    pinEnabled: false,
    active: true,
    createdAt: '2026-08-09T00:00:00Z',
    updatedAt: '2026-08-09T00:00:00Z',
  },
];

const habit: Habit = {
  id: 'habit-1',
  title: 'Brush teeth',
  active: true,
  assignments: [],
  createdAt: '2026-08-09T00:00:00Z',
  updatedAt: '2026-08-09T00:00:00Z',
};

const task: OneOffTask = {
  id: 'task-1',
  childId: 'child-1',
  title: 'Pack library books',
  dueDate: '2026-08-01',
  points: 20,
  status: 'active',
  createdAt: '2026-08-01T00:00:00Z',
};

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <HabitsTasks />
    </QueryClientProvider>,
  );
}

describe('Phase 5 habit and task management', () => {
  beforeEach(() => {
    vi.mocked(routineGroupsApi.list).mockReset().mockResolvedValue([]);
    vi.mocked(householdApi.get).mockReset().mockResolvedValue({
      id: 'household-1',
      name: 'Test household',
      timezone: 'Asia/Jakarta',
      weekStartsOn: 'sunday',
      parentModeTimeoutMinutes: 15,
      version: 1,
    });
    vi.mocked(childrenApi.list).mockReset().mockResolvedValue(children);
    vi.mocked(habitsApi.list).mockReset().mockResolvedValue([]);
    vi.mocked(habitsApi.create).mockReset().mockResolvedValue(habit);
    vi.mocked(habitsApi.assign)
      .mockReset()
      .mockResolvedValue([
        {
          id: 'assignment-1',
          habitId: habit.id,
          childId: children[0]!.id,
          points: 10,
          schedule: { kind: 'daily' },
          effectiveStartDate: '2026-08-09',
          active: true,
        },
      ]);
    vi.mocked(habitsApi.update).mockReset();
    vi.mocked(habitsApi.updateAssignment).mockReset();
    vi.mocked(habitsApi.deactivate).mockReset();
    vi.mocked(tasksApi.list).mockReset().mockResolvedValue([]);
    vi.mocked(tasksApi.create).mockReset();
    vi.mocked(tasksApi.update).mockReset();
    vi.mocked(tasksApi.cancel).mockReset();
  });

  afterEach(cleanup);

  it('progressively creates one recurring definition with atomic child assignments', async () => {
    renderPage();
    await screen.findByRole(
      'heading',
      { name: 'No habits yet' },
      { timeout: 5_000 },
    );
    await userEvent.click(screen.getByRole('button', { name: 'New habit' }));
    await userEvent.type(screen.getByLabelText('Habit name'), 'Read');
    await userEvent.click(screen.getByLabelText('Ari'));
    await userEvent.click(screen.getByLabelText('Bea'));
    await userEvent.selectOptions(
      screen.getByLabelText('Frequency'),
      'weekdays',
    );
    await userEvent.click(screen.getByLabelText('Mon'));
    await userEvent.click(screen.getByLabelText('Wed'));
    await userEvent.click(screen.getByRole('button', { name: 'Create habit' }));

    await waitFor(() => expect(habitsApi.create).toHaveBeenCalledOnce());
    expect(habitsApi.assign).toHaveBeenCalledOnce();
    expect(habitsApi.assign).toHaveBeenCalledWith(
      habit.id,
      expect.objectContaining({
        childIds: ['child-1', 'child-2'],
        schedule: { kind: 'weekdays', weekdays: ['monday', 'wednesday'] },
      }),
    );
  });

  it('retries only atomic assignment after a created habit loses connectivity', async () => {
    vi.mocked(habitsApi.assign)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce([]);
    renderPage();
    await screen.findByRole('heading', { name: 'No habits yet' });
    await userEvent.click(screen.getByRole('button', { name: 'New habit' }));
    await userEvent.type(screen.getByLabelText('Habit name'), 'Read');
    await userEvent.click(screen.getByLabelText('Ari'));
    await userEvent.click(screen.getByRole('button', { name: 'Create habit' }));
    await screen.findByRole('alert');
    await userEvent.click(screen.getByRole('button', { name: 'Create habit' }));

    await waitFor(() => expect(habitsApi.assign).toHaveBeenCalledTimes(2));
    expect(habitsApi.create).toHaveBeenCalledOnce();
  });

  it('makes recurring edits explicitly this-and-future', async () => {
    vi.mocked(habitsApi.list).mockResolvedValue([habit]);
    vi.mocked(habitsApi.update).mockResolvedValue(habit);
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    expect(screen.getByText('This and future dates')).toBeInTheDocument();
    expect(screen.getByText(/Earlier occurrences keep/)).toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('Habit name'));
    await userEvent.type(
      screen.getByLabelText('Habit name'),
      'Brush and floss',
    );
    const effectiveDate =
      screen.getByLabelText<HTMLInputElement>('Effective date').value;
    await userEvent.click(
      screen.getByRole('button', { name: 'Save this and future' }),
    );
    await waitFor(() =>
      expect(habitsApi.update).toHaveBeenCalledWith(
        habit.id,
        expect.objectContaining({
          title: 'Brush and floss',
          effectiveDate,
        }),
      ),
    );
  });

  it('edits assignment points and schedule only from an effective date', async () => {
    const assignment = {
      id: 'assignment-1',
      habitId: habit.id,
      childId: 'child-1',
      points: 10,
      schedule: { kind: 'daily' as const },
      effectiveStartDate: '2026-08-09',
      active: true,
    };
    vi.mocked(habitsApi.list).mockResolvedValue([
      { ...habit, assignments: [assignment] },
    ]);
    vi.mocked(habitsApi.updateAssignment).mockResolvedValue(assignment);
    renderPage();
    await userEvent.click(
      await screen.findByRole('button', { name: 'Edit assignment' }),
    );
    expect(
      screen.getByText(/Earlier occurrences keep their original values/),
    ).toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('Points'));
    await userEvent.type(screen.getByLabelText('Points'), '25');
    await userEvent.selectOptions(
      screen.getByLabelText('Frequency'),
      'weekdays',
    );
    await userEvent.click(screen.getByLabelText('Fri'));
    const effectiveDate =
      screen.getByLabelText<HTMLInputElement>('Effective date').value;
    await userEvent.click(
      screen.getByRole('button', { name: 'Save this and future' }),
    );
    await waitFor(() =>
      expect(habitsApi.updateAssignment).toHaveBeenCalledWith(assignment.id, {
        points: 25,
        schedule: { kind: 'weekdays', weekdays: ['friday'] },
        effectiveDate,
        routineGroupId: null,
        sortOrder: 0,
      }),
    );
  });

  it('labels overdue work neutrally and requires a reason before cancellation', async () => {
    vi.mocked(tasksApi.list).mockResolvedValue([task]);
    vi.mocked(tasksApi.cancel).mockResolvedValue();
    renderPage();
    await userEvent.click(screen.getByRole('tab', { name: 'One-off tasks' }));
    expect(await screen.findByText(/Overdue/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel task' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter a cancellation reason',
    );
    expect(tasksApi.cancel).not.toHaveBeenCalled();
    await userEvent.type(
      screen.getByLabelText('Cancellation reason'),
      'No longer needed',
    );
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() =>
      expect(tasksApi.cancel).toHaveBeenCalledWith(task.id, 'No longer needed'),
    );
  });
});
