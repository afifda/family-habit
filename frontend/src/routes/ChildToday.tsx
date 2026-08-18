import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  todayApi,
  type Completion,
  type Occurrence,
  type Today,
} from '../api/client';
import { ApiError, messageForError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';

type Group = 'todo' | 'waiting' | 'done';

function groupFor(item: Occurrence): Group | null {
  if (item.group === 'to_do') return 'todo';
  if (item.group === 'waiting_for_parent') return 'waiting';
  return item.group === 'done' ? 'done' : null;
}

function dateLabel(date: string, timezone: string) {
  return new Intl.DateTimeFormat(undefined, {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    timeZone: timezone,
  }).format(new Date(`${date}T12:00:00Z`));
}

function typeLabel(type: Occurrence['type']) {
  return type === 'habit' ? 'Habit' : 'One-off task';
}

function dueLabel(item: Occurrence) {
  if (item.status === 'pending_approval') return 'Sent to a parent';
  if (item.status === 'approved') return 'Approved';
  if (item.dueState === 'overdue') {
    const due = item.dueDate
      ? new Intl.DateTimeFormat(undefined, {
          month: 'short',
          day: 'numeric',
          timeZone: 'UTC',
        }).format(new Date(`${item.dueDate}T12:00:00Z`))
      : null;
    return due ? `Still to do · Due ${due}` : 'Still to do';
  }
  if (item.dueState === 'historical') return 'From an earlier day';
  return 'For today';
}

function updateOccurrence(
  current: Today | undefined,
  occurrenceId: string,
  update: Partial<Occurrence>,
) {
  if (!current) return current;
  return {
    ...current,
    occurrences: current.occurrences.map((item) =>
      item.id === occurrenceId ? { ...item, ...update } : item,
    ),
  };
}

export function ChildToday() {
  const { session } = useAuth();
  const childId = session?.actor === 'child' ? session.childId : undefined;
  const queryClient = useQueryClient();
  const queryKey = ['child-today', childId];
  const [selected, setSelected] = useState<Occurrence | null>(null);
  const [detailTrigger, setDetailTrigger] = useState<HTMLButtonElement | null>(
    null,
  );
  const [announcement, setAnnouncement] = useState('');
  const [actionError, setActionError] = useState('');
  const [statusFilter, setStatusFilter] = useState<
    'all' | 'todo' | 'waiting' | 'done'
  >('all');
  const keys = useRef(new Map<string, string>());
  const detailFocusDestination = useRef<Group | null>(null);

  const today = useQuery({
    queryKey,
    queryFn: () => todayApi.get(childId!),
    enabled: Boolean(childId),
  });

  function keyFor(action: string) {
    const existing = keys.current.get(action);
    if (existing) return existing;
    const key = crypto.randomUUID();
    keys.current.set(action, key);
    return key;
  }

  const submit = useMutation({
    mutationFn: (item: Occurrence) =>
      todayApi.submit(item.id, item.version, keyFor(`submit:${item.id}`)),
    onMutate: async (item) => {
      setActionError('');
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<Today>(queryKey);
      queryClient.setQueryData<Today>(queryKey, (current) =>
        updateOccurrence(current, item.id, {
          status: 'pending_approval',
          group: 'waiting_for_parent',
          availableActions: [],
        }),
      );
      setSelected((current) =>
        current?.id === item.id
          ? {
              ...current,
              status: 'pending_approval',
              group: 'waiting_for_parent',
              availableActions: [],
            }
          : current,
      );
      return { previous };
    },
    onSuccess: (completion, item) => {
      keys.current.delete(`submit:${item.id}`);
      queryClient.setQueryData<Today>(queryKey, (current) =>
        updateOccurrence(current, item.id, {
          completionId: completion.id,
          status: completion.occurrenceStatus,
          version: completion.occurrenceVersion,
          group: 'waiting_for_parent',
          availableActions: ['withdraw'],
        }),
      );
      setSelected((current) =>
        current?.id === item.id
          ? {
              ...current,
              completionId: completion.id,
              status: completion.occurrenceStatus,
              version: completion.occurrenceVersion,
              group: 'waiting_for_parent',
              availableActions: ['withdraw'],
            }
          : current,
      );
      setAnnouncement(`${item.title} was sent to a parent.`);
      if (selected?.id === item.id) detailFocusDestination.current = 'waiting';
      if (!selected) {
        requestAnimationFrame(() =>
          document
            .getElementById(`routine-${item.routineGroup?.id ?? 'other'}`)
            ?.focus(),
        );
      }
      void queryClient.invalidateQueries({ queryKey });
    },
    onError: (error, item, context) => {
      queryClient.setQueryData(queryKey, context?.previous);
      setSelected((current) => (current?.id === item.id ? item : current));
      setActionError(
        `Could not send ${item.title}. ${messageForError(error)} Try again.`,
      );
      if (error instanceof ApiError) {
        keys.current.delete(`submit:${item.id}`);
        void queryClient.invalidateQueries({ queryKey });
      }
    },
  });

  const withdraw = useMutation({
    mutationFn: (item: Occurrence) =>
      todayApi.withdraw(
        item.completionId!,
        item.version,
        keyFor(`withdraw:${item.completionId}`),
      ),
    onMutate: async (item) => {
      setActionError('');
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<Today>(queryKey);
      queryClient.setQueryData<Today>(queryKey, (current) =>
        updateOccurrence(current, item.id, {
          status: 'not_started',
          group: 'to_do',
          availableActions: [],
          completionId: null,
        }),
      );
      setSelected((current) =>
        current?.id === item.id
          ? {
              ...current,
              status: 'not_started',
              group: 'to_do',
              availableActions: [],
              completionId: null,
            }
          : current,
      );
      return { previous };
    },
    onSuccess: (completion: Completion, item) => {
      keys.current.delete(`withdraw:${item.completionId}`);
      queryClient.setQueryData<Today>(queryKey, (current) =>
        updateOccurrence(current, item.id, {
          status: completion.occurrenceStatus,
          version: completion.occurrenceVersion,
          group: 'to_do',
          availableActions: ['submit'],
          completionId: null,
        }),
      );
      setAnnouncement(`${item.title} is back in To do.`);
      if (selected?.id === item.id) detailFocusDestination.current = 'todo';
      if (!selected) {
        requestAnimationFrame(() =>
          document
            .getElementById(`routine-${item.routineGroup?.id ?? 'other'}`)
            ?.focus(),
        );
      }
      void queryClient.invalidateQueries({ queryKey });
    },
    onError: (error, item, context) => {
      queryClient.setQueryData(queryKey, context?.previous);
      setSelected((current) => (current?.id === item.id ? item : current));
      setActionError(
        `Could not undo ${item.title}. ${messageForError(error)} Try again.`,
      );
      if (error instanceof ApiError) {
        keys.current.delete(`withdraw:${item.completionId}`);
        void queryClient.invalidateQueries({ queryKey });
      }
    },
  });

  function openDetail(item: Occurrence, trigger: HTMLButtonElement) {
    detailFocusDestination.current = null;
    setDetailTrigger(trigger);
    setSelected(item);
  }

  const closeDetail = useCallback(() => {
    const destinationRoutine = selected
      ? `routine-${selected.routineGroup?.id ?? 'other'}`
      : null;
    const movedBetweenGroups = detailFocusDestination.current !== null;
    setSelected(null);
    requestAnimationFrame(() => {
      if (!movedBetweenGroups && detailTrigger?.isConnected) {
        detailTrigger.focus();
        return;
      }
      if (destinationRoutine) {
        document.getElementById(destinationRoutine)?.focus();
      }
      detailFocusDestination.current = null;
    });
  }, [detailTrigger, selected]);

  const groups = useMemo(() => {
    const result: Record<Group, Occurrence[]> = {
      todo: [],
      waiting: [],
      done: [],
    };
    for (const item of today.data?.occurrences ?? []) {
      const group = groupFor(item);
      if (group) result[group].push(item);
    }
    return result;
  }, [today.data]);

  const routineSections = useMemo(() => {
    const sections = new Map<
      string,
      {
        id: string;
        name: string;
        icon?: string;
        color?: string;
        items: Occurrence[];
      }
    >();
    for (const item of today.data?.occurrences ?? []) {
      const workflow = groupFor(item);
      if (statusFilter !== 'all' && workflow !== statusFilter) continue;
      const routine = item.routineGroup;
      const id = routine?.id ?? 'other';
      const existing = sections.get(id);
      if (existing) existing.items.push(item);
      else
        sections.set(id, {
          id,
          name: routine?.name ?? 'Other',
          icon: routine?.icon,
          color: routine?.color,
          items: [item],
        });
    }
    return [...sections.values()];
  }, [statusFilter, today.data]);

  if (today.isPending) return <TodayLoading />;

  if (today.isError) {
    const forbidden =
      today.error instanceof ApiError && today.error.status === 403;
    return (
      <section className="page today-page">
        <div className="today-state" role="alert">
          <span className="today-state-icon" aria-hidden="true">
            {forbidden ? '🔒' : '↻'}
          </span>
          <h1>
            {forbidden ? 'This is not your Today page' : 'Today did not load'}
          </h1>
          <p>
            {forbidden
              ? 'Switch back to your own profile to see your activities.'
              : 'Check your connection, then try again. Your activities are safe.'}
          </p>
          {!forbidden && (
            <button
              className="button button-primary"
              type="button"
              onClick={() => void today.refetch()}
            >
              Try again
            </button>
          )}
        </div>
      </section>
    );
  }

  const total = groups.todo.length + groups.waiting.length + groups.done.length;
  const done = groups.done.length;

  return (
    <section className="page today-page">
      <div className="today-heading">
        <p className="eyebrow">Your day</p>
        <h1>Today</h1>
        <p className="today-date">
          {dateLabel(today.data.date, today.data.timezone)}
        </p>
        <p className="today-progress">
          <strong>
            {done} of {total}
          </strong>{' '}
          approved today
        </p>
      </div>

      <p className="visually-hidden" aria-live="polite" aria-atomic="true">
        {announcement}
      </p>
      {actionError && (
        <div className="today-action-error" role="alert">
          {actionError}
        </div>
      )}

      <fieldset className="status-filters">
        <legend className="visually-hidden">Filter activities by status</legend>
        {(
          [
            ['all', 'All'],
            ['todo', 'To do'],
            ['waiting', 'Waiting'],
            ['done', 'Done'],
          ] as const
        ).map(([value, label]) => (
          <label key={value}>
            <input
              type="radio"
              name="today-status"
              value={value}
              checked={statusFilter === value}
              onChange={() => setStatusFilter(value)}
            />
            {label} ·{' '}
            {value === 'all'
              ? total
              : groups[value === 'waiting' ? 'waiting' : value].length}
          </label>
        ))}
      </fieldset>

      {total === 0 ? (
        <div className="today-state">
          <span className="today-state-icon" aria-hidden="true">
            ☀
          </span>
          <h2>You’re all caught up</h2>
          <p>There are no habits or tasks for you today.</p>
        </div>
      ) : routineSections.length === 0 ? (
        <div className="today-state">
          <h2>No activities match this filter</h2>
          <button
            className="button button-secondary"
            onClick={() => setStatusFilter('all')}
          >
            Show all
          </button>
        </div>
      ) : (
        routineSections.map((routine) => (
          <TodayRoutine
            key={routine.id}
            routine={routine}
            submittingId={submit.isPending ? submit.variables.id : undefined}
            withdrawingId={
              withdraw.isPending ? withdraw.variables.id : undefined
            }
            onSelect={openDetail}
            onSubmit={(item) => submit.mutate(item)}
            onWithdraw={(item) => withdraw.mutate(item)}
          />
        ))
      )}

      {selected && (
        <OccurrenceDetail
          item={selected}
          submitting={submit.isPending && submit.variables.id === selected.id}
          withdrawing={
            withdraw.isPending && withdraw.variables.id === selected.id
          }
          onClose={closeDetail}
          onSubmit={() => submit.mutate(selected)}
          onWithdraw={() => withdraw.mutate(selected)}
        />
      )}
    </section>
  );
}

function TodayLoading() {
  return (
    <section
      className="page today-page"
      aria-busy="true"
      aria-label="Loading today"
    >
      <div className="today-heading">
        <div className="skeleton skeleton-short" />
        <div className="skeleton skeleton-title" />
        <div className="skeleton skeleton-line" />
      </div>
      <p className="visually-hidden" role="status">
        Loading your activities…
      </p>
      {[1, 2, 3].map((item) => (
        <div className="today-item skeleton-card" key={item} aria-hidden="true">
          <div className="skeleton skeleton-line" />
          <div className="skeleton skeleton-short" />
        </div>
      ))}
    </section>
  );
}

function TodayRoutine({
  routine,
  submittingId,
  withdrawingId,
  onSelect,
  onSubmit,
  onWithdraw,
}: {
  routine: {
    id: string;
    name: string;
    icon?: string;
    color?: string;
    items: Occurrence[];
  };
  submittingId?: string;
  withdrawingId?: string;
  onSelect: (item: Occurrence, trigger: HTMLButtonElement) => void;
  onSubmit: (item: Occurrence) => void;
  onWithdraw: (item: Occurrence) => void;
}) {
  const { items } = routine;
  return (
    <section
      className="today-group routine-section"
      aria-labelledby={`routine-${routine.id}`}
    >
      <h2 id={`routine-${routine.id}`} tabIndex={-1}>
        <span
          className="routine-heading-icon"
          style={routine.color ? { backgroundColor: routine.color } : undefined}
          aria-hidden="true"
        >
          {routine.icon || '•'}
        </span>{' '}
        {routine.name} <span>· {items.length}</span>
      </h2>
      <ul className="today-list">
        {items.map((item) => (
          <li key={item.id}>
            <article className="today-item">
              <button
                className="today-item-detail"
                type="button"
                onClick={(event) => onSelect(item, event.currentTarget)}
                aria-label={`View details for ${item.title}`}
              >
                <span
                  className="today-item-icon"
                  style={
                    item.color ? { backgroundColor: item.color } : undefined
                  }
                  aria-hidden="true"
                >
                  {groupFor(item) === 'done'
                    ? '✓'
                    : item.icon || (item.type === 'habit' ? '★' : '◆')}
                </span>
                <span className="today-item-copy">
                  <strong>{item.title}</strong>
                  <small>
                    {typeLabel(item.type)} · {dueLabel(item)} · {item.points}{' '}
                    {item.points === 1 ? 'point' : 'points'}
                  </small>
                </span>
              </button>
              {item.availableActions.includes('submit') && (
                <button
                  className="button button-primary today-primary"
                  type="button"
                  disabled={submittingId === item.id}
                  aria-busy={submittingId === item.id}
                  onClick={() => onSubmit(item)}
                >
                  {submittingId === item.id ? 'Sending…' : 'I did it'}
                </button>
              )}
              {item.availableActions.includes('withdraw') &&
                item.completionId && (
                  <button
                    className="button button-secondary today-secondary"
                    type="button"
                    disabled={withdrawingId === item.id}
                    aria-busy={withdrawingId === item.id}
                    onClick={() => onWithdraw(item)}
                  >
                    {withdrawingId === item.id
                      ? 'Withdrawing…'
                      : 'Withdraw submission'}
                  </button>
                )}
            </article>
          </li>
        ))}
      </ul>
    </section>
  );
}

function OccurrenceDetail({
  item,
  submitting,
  withdrawing,
  onClose,
  onSubmit,
  onWithdraw,
}: {
  item: Occurrence;
  submitting: boolean;
  withdrawing: boolean;
  onClose: () => void;
  onSubmit: () => void;
  onWithdraw: () => void;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeRef.current?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== 'Tab') return;

      const focusable = Array.from(
        dialogRef.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );
      if (focusable.length === 0) return;
      const first = focusable[0]!;
      const last = focusable[focusable.length - 1]!;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  return (
    <div
      className="dialog-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="today-detail"
        role="dialog"
        aria-modal="true"
        aria-labelledby="today-detail-title"
      >
        <button
          ref={closeRef}
          className="text-button detail-close"
          type="button"
          onClick={onClose}
        >
          ← Today
        </button>
        <span
          className="today-detail-icon"
          style={item.color ? { backgroundColor: item.color } : undefined}
          aria-hidden="true"
        >
          {item.icon || (item.type === 'habit' ? '★' : '◆')}
        </span>
        <p className="eyebrow">{typeLabel(item.type)}</p>
        <h2 id="today-detail-title">{item.title}</h2>
        {item.description && (
          <p className="today-detail-description">{item.description}</p>
        )}
        <p className="today-detail-meta">
          {dueLabel(item)} · {item.points}{' '}
          {item.points === 1 ? 'point' : 'points'}
        </p>
        {item.availableActions.includes('submit') && (
          <button
            className="button button-primary detail-action"
            type="button"
            disabled={submitting}
            aria-busy={submitting}
            onClick={onSubmit}
          >
            {submitting ? 'Sending…' : 'I did it'}
          </button>
        )}
        {item.status === 'pending_approval' && (
          <div className="detail-waiting">
            <strong>Waiting for a parent</strong>
            {item.availableActions.includes('withdraw') &&
              item.completionId && (
                <button
                  className="button button-secondary detail-action"
                  type="button"
                  disabled={withdrawing}
                  aria-busy={withdrawing}
                  onClick={onWithdraw}
                >
                  {withdrawing ? 'Withdrawing…' : 'Withdraw submission'}
                </button>
              )}
          </div>
        )}
        {item.status === 'approved' && (
          <p className="detail-success">
            <strong>Nice work!</strong> Your parent approved this for +
            {item.points} points.
          </p>
        )}
      </div>
    </div>
  );
}
