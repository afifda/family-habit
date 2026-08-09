import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { NavLink, Navigate, Outlet, useNavigate } from 'react-router-dom';

import { profilesApi } from '../api/client';
import { messageForError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';
import { ChildAvatar } from '../components/ChildAvatar';
import { messages } from '../content/messages';

export function ChildShell() {
  const { leaveChild, loading, session } = useAuth();
  const navigate = useNavigate();
  const [switchError, setSwitchError] = useState('');
  const profiles = useQuery({
    queryKey: ['profiles'],
    queryFn: () => profilesApi.list(),
    enabled: session?.actor === 'child' && Boolean(session.childId),
  });

  if (loading)
    return (
      <div className="route-status" role="status">
        Checking your profile…
      </div>
    );
  if (!session) return <Navigate to="/login" replace />;
  if (session.actor !== 'child' || !session.childId)
    return <Navigate to="/" replace />;

  async function switchProfile() {
    setSwitchError('');
    try {
      await leaveChild();
      void navigate('/', { replace: true });
    } catch (error) {
      setSwitchError(messageForError(error));
    }
  }

  const activeProfile = profiles.data?.find(
    (profile) => profile.id === session.childId,
  );

  return (
    <div className="child-shell">
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>
      <header className="site-header">
        <NavLink className="brand" to="/">
          <span className="brand-mark" aria-hidden="true">
            H
          </span>
          <strong>{messages.productName}</strong>
        </NavLink>
        <div className="active-profile">
          {activeProfile ? (
            <ChildAvatar
              avatar={activeProfile.avatar}
              color={activeProfile.color}
            />
          ) : (
            <span aria-hidden="true">●</span>
          )}
          <strong>
            {activeProfile?.nickname ??
              (profiles.isPending ? 'Loading profile…' : 'Child profile')}
          </strong>
          <button
            className="button button-secondary"
            type="button"
            onClick={() => void switchProfile()}
          >
            {messages.switchProfile}
          </button>
        </div>
      </header>
      {profiles.isError && (
        <div className="shell-alert" role="alert">
          Could not load the active profile.{' '}
          <button type="button" onClick={() => void profiles.refetch()}>
            Try again
          </button>
        </div>
      )}
      {switchError && (
        <div className="shell-alert" role="alert">
          {switchError}
        </div>
      )}
      <nav className="child-navigation" aria-label="Child navigation">
        <NavLink to="/child/today">Today</NavLink>
        <NavLink to="/child/points">My points</NavLink>
      </nav>
      <main
        className="main-content child-content"
        id="main-content"
        tabIndex={-1}
      >
        <Outlet />
      </main>
    </div>
  );
}
