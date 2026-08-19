import { FormEvent, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  routineGroupsApi,
  type RoutineGroup,
  type RoutineGroupInput,
} from '../api/client';
import { messageForError } from '../api/errors';

const empty: RoutineGroupInput = {
  name: '',
  icon: '☀',
  color: '#f2b84b',
  startsAtLocal: null,
  endsAtLocal: null,
};

const routineIconChoices = [
  '☀',
  '🌅',
  '🏫',
  '🌆',
  '🌙',
  '⭐',
  '📚',
  '🎨',
  '⚽',
  '🧹',
  '🪥',
  '🛁',
  '🍽️',
  '🛏️',
];

export function RoutineGroups() {
  const queryClient = useQueryClient();
  const groups = useQuery({
    queryKey: ['routine-groups'],
    queryFn: () => routineGroupsApi.list(),
  });
  const [editing, setEditing] = useState<RoutineGroup | 'new' | null>(null);
  const [draft, setDraft] = useState<RoutineGroupInput>(empty);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState('');
  const [archiveGroup, setArchiveGroup] = useState<RoutineGroup | null>(null);
  const [archiveDestination, setArchiveDestination] = useState('');
  const [archiveDate, setArchiveDate] = useState(
    new Intl.DateTimeFormat('en-CA').format(new Date()),
  );
  const key = useRef(crypto.randomUUID());

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['routine-groups'] });
  };
  const save = useMutation({
    mutationFn: () =>
      editing === 'new'
        ? routineGroupsApi.create(draft, key.current)
        : routineGroupsApi.update(editing!.id, draft, editing!.version),
    onSuccess: async () => {
      key.current = crypto.randomUUID();
      setEditing(null);
      setSaved('Routine group saved.');
      await refresh();
    },
    onError: (cause) => setError(messageForError(cause)),
  });
  const archive = useMutation({
    mutationFn: () =>
      routineGroupsApi.archive(archiveGroup!.id, archiveGroup!.version, {
        effectiveFrom: archiveDate,
        moveToRoutineGroupId: archiveDestination || null,
      }),
    onSuccess: async () => {
      setArchiveGroup(null);
      setSaved('Routine archived. Earlier occurrences are unchanged.');
      await refresh();
    },
    onError: (cause) => setError(messageForError(cause)),
  });
  const reorder = useMutation({
    mutationFn: (ordered: RoutineGroup[]) => routineGroupsApi.reorder(ordered),
    onSuccess: refresh,
    onError: (cause) => setError(messageForError(cause)),
  });

  function open(group?: RoutineGroup) {
    setError('');
    setSaved('');
    setDraft(
      group
        ? {
            name: group.name,
            icon: group.icon ?? '',
            color: group.color,
            startsAtLocal: group.startsAtLocal,
            endsAtLocal: group.endsAtLocal,
          }
        : empty,
    );
    setEditing(group ?? 'new');
  }

  function move(index: number, offset: -1 | 1) {
    const ordered = [...(groups.data ?? [])];
    const destination = index + offset;
    if (destination < 0 || destination >= ordered.length) return;
    [ordered[index], ordered[destination]] = [
      ordered[destination]!,
      ordered[index]!,
    ];
    reorder.mutate(ordered);
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    if (!draft.name.trim()) return setError('Enter a routine name.');
    save.mutate();
  }

  if (groups.isPending) return <p role="status">Loading routine groups…</p>;
  if (groups.isError)
    return (
      <div className="notice error" role="alert">
        Routine groups did not load.{' '}
        <button type="button" onClick={() => void groups.refetch()}>
          Try again
        </button>
      </div>
    );

  return (
    <section className="page-stack" aria-labelledby="routine-title">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Parent Mode</p>
          <h1 id="routine-title">Routine groups</h1>
          <p className="page-intro">
            Organize work your way. Time windows are labels, never deadlines.
          </p>
        </div>
        <button
          className="button button-primary"
          type="button"
          onClick={() => open()}
        >
          New group
        </button>
      </div>
      {saved && (
        <div className="notice success" role="status">
          {saved}
        </div>
      )}
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      {(groups.data?.length ?? 0) === 0 ? (
        <div className="empty-state">
          <div>
            <h2>No routine groups yet</h2>
            <p>
              Create Morning, After school, Evening—or anything that fits your
              family. Ungrouped work stays in Other.
            </p>
          </div>
        </div>
      ) : (
        <ol className="routine-list">
          {groups.data?.map((group, index) => (
            <li className="card routine-row" key={group.id}>
              <span
                className="routine-swatch"
                style={{ backgroundColor: group.color }}
                aria-hidden="true"
              >
                {group.icon}
              </span>
              <div className="routine-copy">
                <strong>{group.name}</strong>
                <small>
                  {group.startsAtLocal || group.endsAtLocal
                    ? `${group.startsAtLocal ?? 'Any time'}–${group.endsAtLocal ?? 'Any time'}`
                    : 'No time hint'}
                </small>
              </div>
              <div className="card-actions">
                <button
                  type="button"
                  className="button button-secondary"
                  disabled={index === 0 || reorder.isPending}
                  onClick={() => move(index, -1)}
                  aria-label={`Move ${group.name} up`}
                >
                  ↑
                </button>
                <button
                  type="button"
                  className="button button-secondary"
                  disabled={
                    index === groups.data.length - 1 || reorder.isPending
                  }
                  onClick={() => move(index, 1)}
                  aria-label={`Move ${group.name} down`}
                >
                  ↓
                </button>
                <button
                  type="button"
                  className="button button-secondary"
                  onClick={() => open(group)}
                >
                  Edit
                </button>
                <button
                  type="button"
                  className="button button-danger"
                  disabled={archive.isPending}
                  onClick={() => {
                    setArchiveDestination('');
                    setArchiveGroup(group);
                  }}
                >
                  Archive
                </button>
              </div>
            </li>
          ))}
        </ol>
      )}
      {editing && (
        <form className="card settings-form" onSubmit={submit}>
          <h2>
            {editing === 'new'
              ? 'Create routine group'
              : `Edit ${editing.name}`}
          </h2>
          <label className="form-field">
            Name
            <input
              autoFocus
              required
              maxLength={60}
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
          </label>
          <div className="form-row">
            <fieldset className="icon-picker">
              <legend>Icon</legend>
              <div className="icon-choice-grid">
                {routineIconChoices.map((icon) => (
                  <button
                    key={icon}
                    type="button"
                    className={
                      draft.icon === icon
                        ? 'icon-choice is-selected'
                        : 'icon-choice'
                    }
                    aria-label={`Use ${icon} icon`}
                    aria-pressed={draft.icon === icon}
                    onClick={() => setDraft({ ...draft, icon })}
                  >
                    {icon}
                  </button>
                ))}
              </div>
              <label className="form-field icon-custom-field">
                Custom icon
                <input
                  maxLength={40}
                  value={draft.icon ?? ''}
                  onChange={(e) =>
                    setDraft({ ...draft, icon: e.target.value })
                  }
                />
              </label>
            </fieldset>
            <label className="form-field">
              Color
              <input
                type="color"
                value={draft.color}
                onChange={(e) => setDraft({ ...draft, color: e.target.value })}
              />
            </label>
          </div>
          <div className="form-row">
            <label className="form-field">
              Starts (optional)
              <input
                type="time"
                value={draft.startsAtLocal ?? ''}
                onChange={(e) =>
                  setDraft({ ...draft, startsAtLocal: e.target.value || null })
                }
              />
            </label>
            <label className="form-field">
              Ends (optional)
              <input
                type="time"
                value={draft.endsAtLocal ?? ''}
                onChange={(e) =>
                  setDraft({ ...draft, endsAtLocal: e.target.value || null })
                }
              />
            </label>
          </div>
          <div className="card-actions">
            <button className="button button-primary" disabled={save.isPending}>
              {save.isPending ? 'Saving…' : 'Save group'}
            </button>
            <button
              className="button button-secondary"
              type="button"
              onClick={() => setEditing(null)}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
      {archiveGroup && (
        <form
          className="card settings-form"
          onSubmit={(event) => {
            event.preventDefault();
            archive.mutate();
          }}
        >
          <h2>Archive {archiveGroup.name}?</h2>
          <p>
            Move active work to another routine or Other. Earlier occurrences
            keep their original routine. Progressed work may need to be
            completed or cancelled first.
          </p>
          <label className="form-field">
            Move active work to
            <select
              value={archiveDestination}
              onChange={(event) => setArchiveDestination(event.target.value)}
            >
              <option value="">Other (ungrouped)</option>
              {groups.data
                ?.filter(({ id }) => id !== archiveGroup.id)
                .map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
            </select>
          </label>
          <label className="form-field">
            Effective from
            <input
              required
              type="date"
              value={archiveDate}
              onChange={(event) => setArchiveDate(event.target.value)}
            />
          </label>
          <div className="card-actions">
            <button
              className="button button-danger"
              disabled={archive.isPending}
            >
              {archive.isPending ? 'Archiving…' : 'Archive and move work'}
            </button>
            <button
              className="button button-secondary"
              type="button"
              onClick={() => setArchiveGroup(null)}
            >
              Keep group
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
