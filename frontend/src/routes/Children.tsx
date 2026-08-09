import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  childAvatars,
  childrenApi,
  type Child,
  type ChildAvatar as ChildAvatarName,
  type ChildInput,
} from '../api/client';
import { messageForError } from '../api/errors';
import { ChildAvatar } from '../components/ChildAvatar';
import { FormField, SelectField } from '../components/FormField';

const colors = ['#F5B94C', '#71B790', '#79A9E8', '#D78ED0', '#EF8B72'] as const;

type Draft = ChildInput & { pin: string };
const emptyDraft: Draft = {
  nickname: '',
  avatar: 'fox',
  color: '#F5B94C',
  pin: '',
};

export function Children() {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Child | 'new' | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [formError, setFormError] = useState('');
  const [removePin, setRemovePin] = useState(false);
  const [archiveTarget, setArchiveTarget] = useState<Child | null>(null);
  const children = useQuery({
    queryKey: ['children', showArchived],
    queryFn: () => childrenApi.list(showArchived),
  });

  const save = useMutation({
    mutationFn: async () => {
      const input: ChildInput = {
        nickname: draft.nickname.trim(),
        avatar: draft.avatar,
        color: draft.color,
      };
      if (draft.pin) input.pin = draft.pin;
      else if (editing !== 'new' && removePin) input.pin = null;
      return editing === 'new'
        ? childrenApi.create(input)
        : childrenApi.update(editing!.id, input);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['children'] });
      setEditing(null);
      setDraft(emptyDraft);
      setRemovePin(false);
    },
    onError: (error) => setFormError(messageForError(error)),
  });
  const archive = useMutation({
    mutationFn: (id: string) => childrenApi.archive(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['children'] }),
  });

  function beginEdit(child: Child) {
    setFormError('');
    setEditing(child);
    setRemovePin(false);
    setDraft({
      nickname: child.nickname,
      avatar: child.avatar,
      color: child.color,
      pin: '',
    });
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    if (!draft.nickname.trim()) return setFormError('Enter a nickname.');
    if (draft.pin && !/^\d{4,6}$/.test(draft.pin))
      return setFormError('PIN must contain 4 to 6 numbers.');
    save.mutate();
  }

  return (
    <section className="page" aria-labelledby="children-heading">
      <div className="page-heading-row">
        <div>
          <p className="eyebrow">Parent Mode</p>
          <h1 id="children-heading">Children</h1>
          <p className="page-intro">
            Create a friendly profile for each child using this device.
          </p>
        </div>
        <button
          className="button button-primary"
          type="button"
          onClick={() => {
            setEditing('new');
            setDraft(emptyDraft);
            setFormError('');
          }}
        >
          Add child
        </button>
      </div>

      <label className="toggle-row">
        <input
          type="checkbox"
          checked={showArchived}
          onChange={(event) => setShowArchived(event.target.checked)}
        />
        Show archived profiles
      </label>

      {children.isPending && <p role="status">Loading children…</p>}
      {children.isError && (
        <div className="form-alert" role="alert">
          {messageForError(children.error)}{' '}
          <button type="button" onClick={() => void children.refetch()}>
            Try again
          </button>
        </div>
      )}
      {children.data?.length === 0 && (
        <div className="empty-state">
          <span className="empty-state-icon" aria-hidden="true">
            +
          </span>
          <div>
            <h2>No child profiles yet</h2>
            <p>Add a child to make the shared profile picker ready.</p>
          </div>
        </div>
      )}
      {children.data && children.data.length > 0 && (
        <ul className="child-admin-list">
          {children.data.map((child) => (
            <li key={child.id} className="child-admin-card">
              <ChildAvatar avatar={child.avatar} color={child.color} />
              <div className="child-card-copy">
                <strong>{child.nickname}</strong>
                <small>
                  {child.active
                    ? `${child.pinEnabled ? 'PIN protected' : 'No PIN'} · Active`
                    : 'Archived'}
                </small>
              </div>
              {child.active && (
                <div className="card-actions">
                  <button
                    className="button button-secondary"
                    type="button"
                    onClick={() => beginEdit(child)}
                  >
                    Edit
                  </button>
                  <button
                    className="button button-danger"
                    type="button"
                    disabled={archive.isPending}
                    onClick={() => setArchiveTarget(child)}
                  >
                    Archive
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {archive.isError && (
        <p className="form-alert" role="alert">
          {messageForError(archive.error)}
        </p>
      )}

      {archiveTarget && (
        <div className="dialog-backdrop">
          <section
            className="confirm-dialog"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="archive-heading"
            aria-describedby="archive-description"
          >
            <h2 id="archive-heading">Archive {archiveTarget.nickname}?</h2>
            <p id="archive-description">
              This profile will disappear from the shared picker, but its task
              and points history will be preserved.
            </p>
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                disabled={archive.isPending}
                onClick={() => setArchiveTarget(null)}
              >
                Cancel
              </button>
              <button
                className="button button-danger"
                type="button"
                disabled={archive.isPending}
                autoFocus
                onClick={() =>
                  archive.mutate(archiveTarget.id, {
                    onSuccess: () => setArchiveTarget(null),
                  })
                }
              >
                {archive.isPending ? 'Archiving…' : 'Archive profile'}
              </button>
            </div>
          </section>
        </div>
      )}

      {editing && (
        <section className="editor-card" aria-labelledby="editor-heading">
          <h2 id="editor-heading">
            {editing === 'new' ? 'Add a child' : `Edit ${editing.nickname}`}
          </h2>
          <form className="auth-form" onSubmit={submit} noValidate>
            {formError && (
              <div className="form-alert" role="alert">
                {formError}
              </div>
            )}
            <FormField
              id="child-nickname"
              label="Nickname"
              maxLength={40}
              value={draft.nickname}
              onChange={(event) =>
                setDraft({ ...draft, nickname: event.target.value })
              }
            />
            {editing !== 'new' && editing.pinEnabled && (
              <label className="toggle-row">
                <input
                  type="checkbox"
                  checked={removePin}
                  onChange={(event) => setRemovePin(event.target.checked)}
                />
                Remove the current PIN
              </label>
            )}
            <SelectField
              id="child-avatar"
              label="Avatar"
              value={draft.avatar}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  avatar: event.target.value as ChildAvatarName,
                })
              }
            >
              {childAvatars.map((avatar) => (
                <option key={avatar} value={avatar}>
                  {avatar.charAt(0).toUpperCase() + avatar.slice(1)}
                </option>
              ))}
            </SelectField>
            <fieldset className="color-picker">
              <legend>Profile color</legend>
              <div>
                {colors.map((color) => (
                  <label key={color} style={{ backgroundColor: color }}>
                    <input
                      type="radio"
                      name="color"
                      value={color}
                      checked={draft.color === color}
                      onChange={() => setDraft({ ...draft, color })}
                    />
                    <span className="visually-hidden">{color}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <FormField
              id="child-pin"
              label="Child PIN (optional)"
              hint={
                editing !== 'new' && editing.pinEnabled
                  ? 'Leave blank to keep the current PIN.'
                  : 'Use 4 to 6 numbers, or leave blank for easy entry.'
              }
              inputMode="numeric"
              autoComplete="off"
              value={draft.pin}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  pin: event.target.value.replace(/\D/g, '').slice(0, 6),
                })
              }
            />
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setEditing(null)}
              >
                Cancel
              </button>
              <button
                className="button button-primary"
                disabled={save.isPending}
              >
                {save.isPending ? 'Saving…' : 'Save profile'}
              </button>
            </div>
          </form>
        </section>
      )}
    </section>
  );
}
