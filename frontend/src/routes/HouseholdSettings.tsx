import { FormEvent, useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HouseholdUpdate, householdApi } from '../api/client';
import { messageForError } from '../api/errors';
import { messages } from '../content/messages';

const timezones = [
  'Asia/Jakarta',
  'Europe/Berlin',
  'Europe/London',
  'America/New_York',
  'America/Los_Angeles',
  'Australia/Sydney',
] as const;

export function HouseholdSettings() {
  const queryClient = useQueryClient();
  const household = useQuery({
    queryKey: ['household'],
    queryFn: () => householdApi.get(),
  });
  const [form, setForm] = useState<HouseholdUpdate>({});
  const [parentPin, setParentPin] = useState('');
  const [removeParentPin, setRemoveParentPin] = useState(false);
  const [saved, setSaved] = useState('');

  useEffect(() => {
    if (!household.data) return;
    setForm({
      name: household.data.name,
      timezone: household.data.timezone,
      weekStartsOn: household.data.weekStartsOn,
      parentModeTimeoutMinutes: household.data.parentModeTimeoutMinutes,
    });
  }, [household.data]);

  const update = useMutation({
    mutationFn: () =>
      householdApi.update({
        ...form,
        ...(removeParentPin
          ? { parentPin: null }
          : parentPin
            ? { parentPin }
            : {}),
      }),
    onSuccess: (data) => {
      queryClient.setQueryData(['household'], data);
      setParentPin('');
      setRemoveParentPin(false);
      setSaved(messages.settings.saved);
    },
  });

  if (household.isPending)
    return (
      <div className="route-status" role="status">
        Loading household settings…
      </div>
    );
  if (household.isError)
    return (
      <div className="notice error" role="alert">
        {messages.settings.loadError}{' '}
        <button type="button" onClick={() => void household.refetch()}>
          Try again
        </button>
      </div>
    );

  function submit(event: FormEvent) {
    event.preventDefault();
    setSaved('');
    update.mutate();
  }

  return (
    <section
      className="page-stack settings-page"
      aria-labelledby="settings-title"
    >
      <div className="page-heading">
        <div>
          <p className="eyebrow">Family preferences</p>
          <h1 id="settings-title">{messages.settings.title}</h1>
          <p className="page-intro">{messages.settings.intro}</p>
        </div>
      </div>
      {saved && (
        <div className="notice success" role="status">
          {saved}
        </div>
      )}
      {update.isError && (
        <div className="notice error" role="alert">
          {messageForError(update.error)} {messages.settings.retryHint}
        </div>
      )}
      <form className="settings-form card" onSubmit={submit}>
        <label className="form-field">
          Household name
          <input
            required
            maxLength={80}
            autoComplete="organization"
            value={form.name ?? ''}
            onChange={(event) =>
              setForm((current) => ({ ...current, name: event.target.value }))
            }
          />
        </label>
        <label className="form-field">
          Timezone
          <input
            list="household-timezones"
            required
            value={form.timezone ?? ''}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                timezone: event.target.value,
              }))
            }
          />
          <datalist id="household-timezones">
            {timezones.map((timezone) => (
              <option key={timezone} value={timezone} />
            ))}
          </datalist>
        </label>
        <fieldset className="settings-pin">
          <legend>Parent Mode PIN</legend>
          <label className="form-field">
            Set a new 6-digit PIN
            <input
              inputMode="numeric"
              autoComplete="new-password"
              pattern="[0-9]{6}"
              maxLength={6}
              disabled={removeParentPin}
              value={parentPin}
              onChange={(event) => setParentPin(event.target.value)}
              aria-describedby="parent-pin-help"
            />
          </label>
          <small id="parent-pin-help">
            Leave blank to keep the current PIN. Your account password always
            remains available for Parent Mode.
          </small>
          <label className="toggle-row">
            <input
              type="checkbox"
              checked={removeParentPin}
              onChange={(event) => {
                setRemoveParentPin(event.target.checked);
                if (event.target.checked) setParentPin('');
              }}
            />
            Remove the current Parent Mode PIN
          </label>
        </fieldset>
        <label className="form-field">
          Week starts on
          <select
            value={form.weekStartsOn ?? 'monday'}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                weekStartsOn: event.target.value as 'sunday' | 'monday',
              }))
            }
          >
            <option value="monday">Monday</option>
            <option value="sunday">Sunday</option>
          </select>
        </label>
        <label className="form-field">
          Parent Mode locks after
          <select
            value={form.parentModeTimeoutMinutes ?? 15}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                parentModeTimeoutMinutes: Number(event.target.value) as
                  5 | 15 | 30,
              }))
            }
          >
            <option value="5">5 minutes</option>
            <option value="15">15 minutes</option>
            <option value="30">30 minutes</option>
          </select>
        </label>
        <p className="helper-text">
          Changing the timezone affects future “today” views and reporting
          boundaries; saved history keeps its original household date.
        </p>
        <button
          className="button button-primary settings-submit"
          type="submit"
          disabled={update.isPending}
          aria-busy={update.isPending}
        >
          {update.isPending ? 'Saving…' : 'Save settings'}
        </button>
      </form>
    </section>
  );
}
