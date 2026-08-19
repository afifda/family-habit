import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, Navigate, useNavigate } from 'react-router-dom';

import { profileSummaryApi, profilesApi, type Profile } from '../api/client';
import { messageForError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';
import { ChildAvatar } from '../components/ChildAvatar';
import { FormField } from '../components/FormField';
import { messages } from '../content/messages';

export function ProfilePicker() {
  const { enterChild, loading, session } = useAuth();
  const navigate = useNavigate();
  const [selected, setSelected] = useState<Profile | null>(null);
  const [pin, setPin] = useState('');
  const [error, setError] = useState('');
  const [entering, setEntering] = useState(false);
  const profiles = useQuery({
    queryKey: ['profiles'],
    queryFn: () => profilesApi.list(),
    enabled: Boolean(session),
  });
  const overview = useQuery({
    queryKey: ['profiles', 'summary'],
    queryFn: () => profileSummaryApi.get(),
    enabled: Boolean(session) && session?.actor !== 'child',
  });

  if (loading)
    return (
      <div className="route-status" role="status">
        Loading profiles…
      </div>
    );
  if (!session) return <Navigate to="/login" replace />;
  if (session.actor === 'child') return <Navigate to="/child/today" replace />;

  async function choose(child: Profile) {
    if (child.pinRequired) {
      setSelected(child);
      setPin('');
      setError('');
      return;
    }
    await enter(child);
  }

  async function enter(child: Profile) {
    setEntering(true);
    setError('');
    try {
      await enterChild({ childId: child.id, ...(pin ? { pin } : {}) });
      void navigate('/child/today', { replace: true });
    } catch (caught) {
      setError(messageForError(caught));
    } finally {
      setEntering(false);
    }
  }

  const todayPointsByChild = new Map(
    (overview.data?.children ?? []).map((child) => [
      child.childId,
      {
        approved: child.approvedPointsToday,
        waiting: child.waitingPointsToday,
      },
    ]),
  );

  return (
    <main className="profile-page" id="main-content">
      <section className="profile-card" aria-labelledby="profile-heading">
        <div className="profile-brand">
          <span className="brand-mark brand-mark-large" aria-hidden="true">
            H
          </span>
          <p className="eyebrow">{messages.tagline}</p>
        </div>
        <h1 id="profile-heading">Who is using Habit Home?</h1>
        <p className="page-intro">Choose your profile to see your own space.</p>
        {profiles.isPending && <p role="status">Loading family profiles…</p>}
        {profiles.isError && (
          <div className="form-alert" role="alert">
            {messageForError(profiles.error)}{' '}
            <button type="button" onClick={() => void profiles.refetch()}>
              Try again
            </button>
          </div>
        )}
        {profiles.data?.length === 0 && (
          <div className="empty-state profile-empty">
            <span className="empty-state-icon" aria-hidden="true">
              ★
            </span>
            <div>
              <h2>No child profiles yet</h2>
              <p>Enter Parent Mode to add the first child.</p>
            </div>
          </div>
        )}
        {!profiles.isPending && !profiles.isError && (
          <div className="profile-grid" aria-label="Family profiles">
            {profiles.data?.map((child) => {
              const points = todayPointsByChild.get(child.id);
              const pointLabel = points
                ? `, ${points.approved} approved points and ${points.waiting} waiting points today`
                : '';
              return (
                <button
                  className="profile-choice"
                  type="button"
                  key={child.id}
                  disabled={entering}
                  onClick={() => void choose(child)}
                  aria-label={`${child.nickname}${pointLabel}${child.pinRequired ? ', PIN required' : ''}`}
                >
                  <ChildAvatar
                    avatar={child.avatar}
                    color={child.color}
                    size="large"
                  />
                  <strong>{child.nickname}</strong>
                  {points && (
                    <span className="profile-points" aria-hidden="true">
                      <span>
                        <strong>{points.approved}</strong>
                        <small>Approved today</small>
                      </span>
                      <span>
                        <strong>{points.waiting}</strong>
                        <small>Waiting</small>
                      </span>
                    </span>
                  )}
                  <small>
                    {child.pinRequired ? 'PIN required' : 'Tap to enter'}
                  </small>
                </button>
              );
            })}
            <Link
              className="profile-choice"
              to="/parent/unlock"
              aria-label="Parent Mode, manage the family"
            >
              <span
                className="avatar avatar-parent avatar-large"
                aria-hidden="true"
              >
                P
              </span>
              <strong>{messages.parentMode}</strong>
              <small>Manage the family</small>
            </Link>
          </div>
        )}
        {selected && (
          <form
            className="pin-panel"
            onSubmit={(event) => {
              event.preventDefault();
              void enter(selected);
            }}
          >
            <ChildAvatar avatar={selected.avatar} color={selected.color} />
            <h2>Enter {selected.nickname}’s PIN</h2>
            {error && (
              <div className="form-alert" role="alert">
                {error}
              </div>
            )}
            <FormField
              id="profile-pin"
              label="PIN"
              inputMode="numeric"
              pattern="[0-9]*"
              type="password"
              autoComplete="off"
              autoFocus
              value={pin}
              onChange={(event) =>
                setPin(event.target.value.replace(/\D/g, '').slice(0, 6))
              }
            />
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setSelected(null)}
              >
                Cancel
              </button>
              <button
                className="button button-primary"
                disabled={entering || pin.length < 4}
              >
                {entering ? 'Checking…' : 'Enter profile'}
              </button>
            </div>
          </form>
        )}
        {!selected && error && (
          <p className="form-alert" role="alert">
            {error}
          </p>
        )}
      </section>
    </main>
  );
}
