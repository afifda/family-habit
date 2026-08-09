import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type FormEvent, useState } from 'react';

import {
  childrenApi,
  habitsApi,
  householdApi,
  tasksApi,
  type Assignment,
  type Child,
  type Habit,
  type OneOffTask,
  type Schedule,
  type Weekday,
} from '../api/client';
import { messageForError } from '../api/errors';
import { FormField, SelectField } from '../components/FormField';

const weekdays: { value: Weekday; label: string }[] = [
  { value: 'monday', label: 'Mon' },
  { value: 'tuesday', label: 'Tue' },
  { value: 'wednesday', label: 'Wed' },
  { value: 'thursday', label: 'Thu' },
  { value: 'friday', label: 'Fri' },
  { value: 'saturday', label: 'Sat' },
  { value: 'sunday', label: 'Sun' },
];

const localToday = (timezone = 'Asia/Jakarta') =>
  new Intl.DateTimeFormat('en-CA', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date());

type HabitDraft = {
  title: string;
  description: string;
  childIds: string[];
  points: string;
  scheduleKind: 'daily' | 'weekdays';
  weekdays: Weekday[];
  effectiveDate: string;
};

const emptyHabit = (timezone?: string): HabitDraft => ({
  title: '',
  description: '',
  childIds: [],
  points: '10',
  scheduleKind: 'daily',
  weekdays: [],
  effectiveDate: localToday(timezone),
});

type TaskDraft = {
  childId: string;
  title: string;
  description: string;
  dueDate: string;
  points: string;
};

const emptyTask = (timezone?: string): TaskDraft => ({
  childId: '',
  title: '',
  description: '',
  dueDate: localToday(timezone),
  points: '10',
});

function scheduleFrom(draft: HabitDraft): Schedule {
  return draft.scheduleKind === 'daily'
    ? { kind: 'daily' }
    : { kind: 'weekdays', weekdays: draft.weekdays };
}

function scheduleLabel(schedule: Schedule) {
  if (schedule.kind === 'daily') return 'Every day';
  return schedule.weekdays
    .map((day) => weekdays.find(({ value }) => value === day)?.label ?? day)
    .join(', ');
}

function childName(children: Child[], id: string) {
  return (
    children.find((child) => child.id === id)?.nickname ?? 'Archived child'
  );
}

export function HabitsTasks() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<'habits' | 'tasks'>('habits');
  const [habitEditor, setHabitEditor] = useState<'new' | Habit | null>(null);
  const [taskEditor, setTaskEditor] = useState<'new' | OneOffTask | null>(null);
  const [assignmentEditor, setAssignmentEditor] = useState<Assignment | null>(
    null,
  );
  const [habitDraft, setHabitDraft] = useState(emptyHabit);
  const [taskDraft, setTaskDraft] = useState(emptyTask);
  const [formError, setFormError] = useState('');
  const [showHistory, setShowHistory] = useState(false);
  const [cancellationReason, setCancellationReason] = useState('');
  const [pendingHabitId, setPendingHabitId] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<
    { kind: 'habit'; item: Habit } | { kind: 'task'; item: OneOffTask } | null
  >(null);

  const children = useQuery({
    queryKey: ['children', false],
    queryFn: () => childrenApi.list(false),
  });
  const household = useQuery({
    queryKey: ['household'],
    queryFn: () => householdApi.get(),
  });
  const habits = useQuery({
    queryKey: ['habits'],
    queryFn: () => habitsApi.list(),
  });
  const tasks = useQuery({
    queryKey: ['tasks'],
    queryFn: () => tasksApi.list(),
  });

  const saveHabit = useMutation({
    mutationFn: async () => {
      const presentation = {
        title: habitDraft.title.trim(),
        description: habitDraft.description.trim(),
      };
      if (habitEditor !== 'new') {
        const input = {
          ...presentation,
          effectiveDate: habitDraft.effectiveDate,
        };
        return habitEditor!.version
          ? habitsApi.update(habitEditor!.id, input, habitEditor!.version)
          : habitsApi.update(habitEditor!.id, input);
      }
      let habitId = pendingHabitId;
      let habit: Habit;
      if (habitId) {
        habit = (habits.data ?? []).find(({ id }) => id === habitId) ?? {
          id: habitId,
          ...presentation,
          active: true,
          assignments: [],
          createdAt: '',
          updatedAt: '',
        };
      } else {
        habit = await habitsApi.create(presentation);
        habitId = habit.id;
        setPendingHabitId(habitId);
      }
      await habitsApi.assign(habitId, {
        childIds: habitDraft.childIds,
        points: Number(habitDraft.points),
        schedule: scheduleFrom(habitDraft),
        effectiveStartDate: habitDraft.effectiveDate,
      });
      return habit;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['habits'] });
      setPendingHabitId(null);
      setHabitEditor(null);
    },
    onError: (error) => setFormError(messageForError(error)),
  });
  const deactivateHabit = useMutation({
    mutationFn: (habit: Habit) =>
      habitsApi.deactivate(
        habit.id,
        localToday(household.data?.timezone),
        habit.version,
      ),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['habits'] });
      setConfirm(null);
    },
  });
  const saveAssignment = useMutation({
    mutationFn: () => {
      const input = {
        points: Number(habitDraft.points),
        schedule: scheduleFrom(habitDraft),
        effectiveDate: habitDraft.effectiveDate,
      };
      return assignmentEditor!.version
        ? habitsApi.updateAssignment(
            assignmentEditor!.id,
            input,
            assignmentEditor!.version,
          )
        : habitsApi.updateAssignment(assignmentEditor!.id, input);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['habits'] });
      setAssignmentEditor(null);
    },
    onError: (error) => setFormError(messageForError(error)),
  });
  const saveTask = useMutation({
    mutationFn: () => {
      const input = {
        title: taskDraft.title.trim(),
        description: taskDraft.description.trim(),
        dueDate: taskDraft.dueDate,
        points: Number(taskDraft.points),
      };
      return taskEditor === 'new'
        ? tasksApi.create({ ...input, childId: taskDraft.childId })
        : taskEditor!.version
          ? tasksApi.update(taskEditor!.id, input, taskEditor!.version)
          : tasksApi.update(taskEditor!.id, input);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['tasks'] });
      setTaskEditor(null);
    },
    onError: (error) => setFormError(messageForError(error)),
  });
  const cancelTask = useMutation({
    mutationFn: (task: OneOffTask) =>
      task.version
        ? tasksApi.cancel(task.id, cancellationReason.trim(), task.version)
        : tasksApi.cancel(task.id, cancellationReason.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['tasks'] });
      setConfirm(null);
    },
  });

  function openNewHabit() {
    setFormError('');
    setPendingHabitId(null);
    setHabitDraft(emptyHabit(household.data?.timezone));
    setHabitEditor('new');
  }

  function openHabit(habit: Habit) {
    setFormError('');
    setHabitDraft({
      ...emptyHabit(household.data?.timezone),
      title: habit.title,
      description: habit.description ?? '',
    });
    setHabitEditor(habit);
  }

  function submitHabit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    if (!habitDraft.title.trim()) return setFormError('Enter a habit name.');
    const points = Number(habitDraft.points);
    if (habitEditor === 'new' && habitDraft.childIds.length === 0)
      return setFormError('Choose at least one child.');
    if (
      habitEditor === 'new' &&
      (!Number.isInteger(points) || points < 1 || points > 10_000)
    )
      return setFormError('Points must be a whole number from 1 to 10,000.');
    if (
      habitEditor === 'new' &&
      habitDraft.scheduleKind === 'weekdays' &&
      habitDraft.weekdays.length === 0
    )
      return setFormError('Choose at least one weekday.');
    saveHabit.mutate();
  }

  function openAssignment(assignment: Assignment) {
    setFormError('');
    setHabitDraft({
      ...emptyHabit(household.data?.timezone),
      points: String(assignment.points),
      scheduleKind: assignment.schedule.kind,
      weekdays:
        assignment.schedule.kind === 'weekdays'
          ? assignment.schedule.weekdays
          : [],
    });
    setAssignmentEditor(assignment);
  }

  function submitAssignment(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    const points = Number(habitDraft.points);
    if (!Number.isInteger(points) || points < 1 || points > 10_000)
      return setFormError('Points must be a whole number from 1 to 10,000.');
    if (
      habitDraft.scheduleKind === 'weekdays' &&
      habitDraft.weekdays.length === 0
    )
      return setFormError('Choose at least one weekday.');
    saveAssignment.mutate();
  }

  function openNewTask() {
    setFormError('');
    setTaskDraft({
      ...emptyTask(household.data?.timezone),
      childId: children.data?.[0]?.id ?? '',
    });
    setTaskEditor('new');
  }

  function openTask(task: OneOffTask) {
    setFormError('');
    setTaskDraft({
      childId: task.childId,
      title: task.title,
      description: task.description ?? '',
      dueDate: task.dueDate,
      points: String(task.points),
    });
    setTaskEditor(task);
  }

  function submitTask(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    const points = Number(taskDraft.points);
    if (!taskDraft.title.trim()) return setFormError('Enter a task name.');
    if (!taskDraft.childId) return setFormError('Choose a child.');
    if (!taskDraft.dueDate) return setFormError('Choose a due date.');
    if (!Number.isInteger(points) || points < 1 || points > 10_000)
      return setFormError('Points must be a whole number from 1 to 10,000.');
    saveTask.mutate();
  }

  const loading = children.isPending || habits.isPending || tasks.isPending;
  const loadError = children.error ?? habits.error ?? tasks.error;
  const activeTasks =
    tasks.data?.filter((task) => task.status === 'active') ?? [];
  const cancelledTasks =
    tasks.data?.filter((task) => task.status === 'cancelled') ?? [];
  const activeHabits = habits.data?.filter((habit) => habit.active) ?? [];
  const inactiveHabits = habits.data?.filter((habit) => !habit.active) ?? [];

  return (
    <section className="page" aria-labelledby="work-heading">
      <div className="page-heading-row">
        <div>
          <p className="eyebrow">Parent Mode</p>
          <h1 id="work-heading">Habits &amp; tasks</h1>
          <p className="page-intro">
            Plan recurring routines or add something that only needs doing once.
          </p>
        </div>
        <button
          className="button button-primary"
          type="button"
          onClick={tab === 'habits' ? openNewHabit : openNewTask}
        >
          {tab === 'habits' ? 'New habit' : 'New task'}
        </button>
      </div>

      <div className="segment-control" role="tablist" aria-label="Work type">
        <button
          role="tab"
          aria-selected={tab === 'habits'}
          onClick={() => setTab('habits')}
        >
          Habits
        </button>
        <button
          role="tab"
          aria-selected={tab === 'tasks'}
          onClick={() => setTab('tasks')}
        >
          One-off tasks
        </button>
      </div>
      <label className="toggle-row history-toggle">
        <input
          type="checkbox"
          checked={showHistory}
          onChange={(event) => setShowHistory(event.target.checked)}
        />
        Show inactive and cancelled items
      </label>

      {loading && <p role="status">Loading habits and tasks…</p>}
      {loadError && (
        <div className="form-alert" role="alert">
          {messageForError(loadError)}{' '}
          <button
            type="button"
            onClick={() => {
              void habits.refetch();
              void tasks.refetch();
              void children.refetch();
            }}
          >
            Try again
          </button>
        </div>
      )}

      {!loading && !loadError && tab === 'habits' && (
        <div role="tabpanel">
          {activeHabits.length === 0 ? (
            <div className="empty-state">
              <span className="empty-state-icon" aria-hidden="true">
                ↻
              </span>
              <div>
                <h2>No habits yet</h2>
                <p>
                  Create a routine, choose who does it, and set its schedule.
                </p>
              </div>
            </div>
          ) : (
            <ul className="work-list">
              {activeHabits.map((habit) => (
                <li className="work-card" key={habit.id}>
                  <div className="work-card-copy">
                    <strong>{habit.title}</strong>
                    <small>
                      {habit.active
                        ? `${habit.assignments?.length ?? 0} active assignment${habit.assignments?.length === 1 ? '' : 's'}`
                        : 'Inactive'}
                    </small>
                    {habit.description && <p>{habit.description}</p>}
                  </div>
                  <div className="card-actions">
                    <button
                      className="button button-secondary"
                      type="button"
                      onClick={() => openHabit(habit)}
                    >
                      Edit
                    </button>
                    {habit.active && (
                      <button
                        className="button button-danger"
                        type="button"
                        onClick={() => {
                          setFormError('');
                          setConfirm({ kind: 'habit', item: habit });
                        }}
                      >
                        Deactivate
                      </button>
                    )}
                  </div>
                  {habit.assignments && habit.assignments.length > 0 && (
                    <ul className="assignment-list">
                      {habit.assignments.map((assignment) => (
                        <li key={assignment.id}>
                          <span>
                            {childName(children.data ?? [], assignment.childId)}
                          </span>
                          <span>
                            {scheduleLabel(assignment.schedule)} ·{' '}
                            {assignment.points} points · from{' '}
                            {assignment.effectiveStartDate}
                          </span>
                          <button
                            className="text-button"
                            type="button"
                            onClick={() => openAssignment(assignment)}
                          >
                            Edit assignment
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </li>
              ))}
            </ul>
          )}
          {showHistory && inactiveHabits.length > 0 && (
            <section
              className="history-section"
              aria-labelledby="inactive-habits-heading"
            >
              <h2 id="inactive-habits-heading">Inactive habits</h2>
              <ul className="work-list">
                {inactiveHabits.map((habit) => (
                  <li className="work-card muted-card" key={habit.id}>
                    <div className="work-card-copy">
                      <strong>{habit.title}</strong>
                      <small>Inactive · history preserved</small>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>
      )}

      {!loading && !loadError && tab === 'tasks' && (
        <div role="tabpanel">
          {activeTasks.length === 0 ? (
            <div className="empty-state">
              <span className="empty-state-icon" aria-hidden="true">
                ✓
              </span>
              <div>
                <h2>No one-off tasks</h2>
                <p>
                  Add a dated job for one child. Unfinished tasks remain
                  available when overdue.
                </p>
              </div>
            </div>
          ) : (
            <ul className="work-list">
              {activeTasks.map((task) => (
                <li className="work-card" key={task.id}>
                  <div className="work-card-copy">
                    <strong>{task.title}</strong>
                    <small>
                      {childName(children.data ?? [], task.childId)} · due{' '}
                      {task.dueDate} · {task.points} points
                      {task.dueDate < localToday(household.data?.timezone)
                        ? ' · Overdue'
                        : ''}
                    </small>
                    {task.description && <p>{task.description}</p>}
                  </div>
                  <div className="card-actions">
                    <button
                      className="button button-secondary"
                      type="button"
                      onClick={() => openTask(task)}
                    >
                      Edit
                    </button>
                    <button
                      className="button button-danger"
                      type="button"
                      onClick={() => {
                        setFormError('');
                        setCancellationReason('');
                        setConfirm({ kind: 'task', item: task });
                      }}
                    >
                      Cancel task
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {showHistory && cancelledTasks.length > 0 && (
            <section
              className="history-section"
              aria-labelledby="cancelled-tasks-heading"
            >
              <h2 id="cancelled-tasks-heading">Cancelled tasks</h2>
              <ul className="work-list">
                {cancelledTasks.map((task) => (
                  <li className="work-card muted-card" key={task.id}>
                    <div className="work-card-copy">
                      <strong>{task.title}</strong>
                      <small>
                        {childName(children.data ?? [], task.childId)} ·
                        cancelled
                      </small>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>
      )}

      {habitEditor && (
        <section
          className="editor-card work-editor"
          aria-labelledby="habit-editor-heading"
        >
          <h2 id="habit-editor-heading">
            {habitEditor === 'new'
              ? 'Create a habit'
              : `Edit ${habitEditor.title}`}
          </h2>
          {habitEditor !== 'new' && (
            <div className="future-notice">
              <strong>This and future dates</strong>
              <p>
                Your change starts on the effective date. Earlier occurrences
                keep their original title and points.
              </p>
            </div>
          )}
          <form className="auth-form" onSubmit={submitHabit} noValidate>
            {formError && (
              <div className="form-alert" role="alert">
                {formError}
              </div>
            )}
            <fieldset>
              <legend>1. Habit details</legend>
              <FormField
                id="habit-title"
                label="Habit name"
                maxLength={120}
                value={habitDraft.title}
                onChange={(event) =>
                  setHabitDraft({ ...habitDraft, title: event.target.value })
                }
              />
              <FormField
                id="habit-description"
                label="Description (optional)"
                maxLength={500}
                value={habitDraft.description}
                onChange={(event) =>
                  setHabitDraft({
                    ...habitDraft,
                    description: event.target.value,
                  })
                }
              />
            </fieldset>
            {habitEditor === 'new' && (
              <>
                <fieldset>
                  <legend>2. Who does it?</legend>
                  <div className="choice-grid">
                    {children.data?.map((child) => (
                      <label key={child.id}>
                        <input
                          type="checkbox"
                          checked={habitDraft.childIds.includes(child.id)}
                          onChange={() =>
                            setHabitDraft({
                              ...habitDraft,
                              childIds: habitDraft.childIds.includes(child.id)
                                ? habitDraft.childIds.filter(
                                    (id) => id !== child.id,
                                  )
                                : [...habitDraft.childIds, child.id],
                            })
                          }
                        />
                        {child.nickname}
                      </label>
                    ))}
                  </div>
                </fieldset>
                <fieldset>
                  <legend>3. Points and schedule</legend>
                  <FormField
                    id="habit-points"
                    label="Points"
                    type="number"
                    min={1}
                    max={10000}
                    value={habitDraft.points}
                    onChange={(event) =>
                      setHabitDraft({
                        ...habitDraft,
                        points: event.target.value,
                      })
                    }
                  />
                  <SelectField
                    id="habit-frequency"
                    label="Frequency"
                    value={habitDraft.scheduleKind}
                    onChange={(event) =>
                      setHabitDraft({
                        ...habitDraft,
                        scheduleKind: event.target
                          .value as HabitDraft['scheduleKind'],
                      })
                    }
                  >
                    <option value="daily">Every day</option>
                    <option value="weekdays">Selected weekdays</option>
                  </SelectField>
                  {habitDraft.scheduleKind === 'weekdays' && (
                    <fieldset className="weekday-picker">
                      <legend>Days</legend>
                      {weekdays.map((day) => (
                        <label key={day.value}>
                          <input
                            type="checkbox"
                            checked={habitDraft.weekdays.includes(day.value)}
                            onChange={() =>
                              setHabitDraft({
                                ...habitDraft,
                                weekdays: habitDraft.weekdays.includes(
                                  day.value,
                                )
                                  ? habitDraft.weekdays.filter(
                                      (value) => value !== day.value,
                                    )
                                  : [...habitDraft.weekdays, day.value],
                              })
                            }
                          />
                          {day.label}
                        </label>
                      ))}
                    </fieldset>
                  )}
                </fieldset>
              </>
            )}
            <FormField
              id="habit-effective-date"
              label={habitEditor === 'new' ? 'Start date' : 'Effective date'}
              type="date"
              value={habitDraft.effectiveDate}
              onChange={(event) =>
                setHabitDraft({
                  ...habitDraft,
                  effectiveDate: event.target.value,
                })
              }
            />
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setHabitEditor(null)}
              >
                Cancel
              </button>
              <button
                className="button button-primary"
                type="submit"
                disabled={saveHabit.isPending}
              >
                {saveHabit.isPending
                  ? 'Saving…'
                  : habitEditor === 'new'
                    ? 'Create habit'
                    : 'Save this and future'}
              </button>
            </div>
          </form>
        </section>
      )}

      {taskEditor && (
        <section
          className="editor-card work-editor"
          aria-labelledby="task-editor-heading"
        >
          <h2 id="task-editor-heading">
            {taskEditor === 'new'
              ? 'Create a one-off task'
              : `Edit ${taskEditor.title}`}
          </h2>
          <form className="auth-form" onSubmit={submitTask} noValidate>
            {formError && (
              <div className="form-alert" role="alert">
                {formError}
              </div>
            )}
            <SelectField
              id="task-child"
              label="Child"
              value={taskDraft.childId}
              disabled={taskEditor !== 'new'}
              onChange={(event) =>
                setTaskDraft({ ...taskDraft, childId: event.target.value })
              }
            >
              <option value="">Choose a child</option>
              {children.data?.map((child) => (
                <option key={child.id} value={child.id}>
                  {child.nickname}
                </option>
              ))}
            </SelectField>
            <FormField
              id="task-title"
              label="Task name"
              maxLength={120}
              value={taskDraft.title}
              onChange={(event) =>
                setTaskDraft({ ...taskDraft, title: event.target.value })
              }
            />
            <FormField
              id="task-description"
              label="Description (optional)"
              maxLength={500}
              value={taskDraft.description}
              onChange={(event) =>
                setTaskDraft({ ...taskDraft, description: event.target.value })
              }
            />
            <div className="form-row">
              <FormField
                id="task-date"
                label="Due date"
                type="date"
                value={taskDraft.dueDate}
                onChange={(event) =>
                  setTaskDraft({ ...taskDraft, dueDate: event.target.value })
                }
              />
              <FormField
                id="task-points"
                label="Points"
                type="number"
                min={1}
                max={10000}
                value={taskDraft.points}
                onChange={(event) =>
                  setTaskDraft({ ...taskDraft, points: event.target.value })
                }
              />
            </div>
            <p className="helper-inline">
              If it is not finished by its due date, it will stay visible as
              overdue.
            </p>
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setTaskEditor(null)}
              >
                Cancel
              </button>
              <button
                className="button button-primary"
                type="submit"
                disabled={saveTask.isPending}
              >
                {saveTask.isPending
                  ? 'Saving…'
                  : taskEditor === 'new'
                    ? 'Create task'
                    : 'Save task'}
              </button>
            </div>
          </form>
        </section>
      )}

      {assignmentEditor && (
        <section
          className="editor-card work-editor"
          aria-labelledby="assignment-editor-heading"
        >
          <h2 id="assignment-editor-heading">
            Edit {childName(children.data ?? [], assignmentEditor.childId)}’s
            assignment
          </h2>
          <div className="future-notice">
            <strong>This and future dates</strong>
            <p>
              Points and schedule change from the effective date. Earlier
              occurrences keep their original values.
            </p>
          </div>
          <form className="auth-form" onSubmit={submitAssignment} noValidate>
            {formError && (
              <div className="form-alert" role="alert">
                {formError}
              </div>
            )}
            <FormField
              id="assignment-points"
              label="Points"
              type="number"
              min={1}
              max={10000}
              value={habitDraft.points}
              onChange={(event) =>
                setHabitDraft({ ...habitDraft, points: event.target.value })
              }
            />
            <SelectField
              id="assignment-frequency"
              label="Frequency"
              value={habitDraft.scheduleKind}
              onChange={(event) =>
                setHabitDraft({
                  ...habitDraft,
                  scheduleKind: event.target
                    .value as HabitDraft['scheduleKind'],
                })
              }
            >
              <option value="daily">Every day</option>
              <option value="weekdays">Selected weekdays</option>
            </SelectField>
            {habitDraft.scheduleKind === 'weekdays' && (
              <fieldset className="weekday-picker">
                <legend>Days</legend>
                {weekdays.map((day) => (
                  <label key={day.value}>
                    <input
                      type="checkbox"
                      checked={habitDraft.weekdays.includes(day.value)}
                      onChange={() =>
                        setHabitDraft({
                          ...habitDraft,
                          weekdays: habitDraft.weekdays.includes(day.value)
                            ? habitDraft.weekdays.filter(
                                (value) => value !== day.value,
                              )
                            : [...habitDraft.weekdays, day.value],
                        })
                      }
                    />
                    {day.label}
                  </label>
                ))}
              </fieldset>
            )}
            <FormField
              id="assignment-effective-date"
              label="Effective date"
              type="date"
              value={habitDraft.effectiveDate}
              onChange={(event) =>
                setHabitDraft({
                  ...habitDraft,
                  effectiveDate: event.target.value,
                })
              }
            />
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setAssignmentEditor(null)}
              >
                Cancel
              </button>
              <button
                className="button button-primary"
                type="submit"
                disabled={saveAssignment.isPending}
              >
                {saveAssignment.isPending ? 'Saving…' : 'Save this and future'}
              </button>
            </div>
          </form>
        </section>
      )}

      {confirm && (
        <div className="dialog-backdrop">
          <section
            className="confirm-dialog"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="remove-work-heading"
          >
            <h2 id="remove-work-heading">
              {confirm.kind === 'habit'
                ? `Deactivate ${confirm.item.title}?`
                : `Cancel ${confirm.item.title}?`}
            </h2>
            <p>
              {confirm.kind === 'habit'
                ? 'This stops this habit from today onward. Past and completed occurrences stay unchanged.'
                : 'This removes the unfinished task from the child’s list while preserving its record.'}
            </p>
            {confirm.kind === 'task' && (
              <FormField
                id="cancellation-reason"
                label="Cancellation reason"
                maxLength={500}
                value={cancellationReason}
                onChange={(event) => setCancellationReason(event.target.value)}
              />
            )}
            {(deactivateHabit.isError || cancelTask.isError) && (
              <p className="form-alert" role="alert">
                {messageForError(deactivateHabit.error ?? cancelTask.error)}
              </p>
            )}
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setConfirm(null)}
              >
                Keep it
              </button>
              <button
                className="button button-danger"
                type="button"
                disabled={deactivateHabit.isPending || cancelTask.isPending}
                onClick={() =>
                  confirm.kind === 'habit'
                    ? deactivateHabit.mutate(confirm.item)
                    : cancellationReason.trim()
                      ? cancelTask.mutate(confirm.item)
                      : setFormError('Enter a cancellation reason.')
                }
              >
                Confirm
              </button>
            </div>
            {confirm.kind === 'task' && formError && (
              <p className="field-error" role="alert">
                {formError}
              </p>
            )}
          </section>
        </div>
      )}
    </section>
  );
}
