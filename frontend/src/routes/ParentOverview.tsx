import { useQueries, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';

import { overviewApi, pointsApi } from '../api/client';
import { ChildAvatar } from '../components/ChildAvatar';
import { messages } from '../content/messages';

export function ParentOverview() {
  const overview = useQuery({
    queryKey: ['parent', 'overview'],
    queryFn: () => overviewApi.get(),
  });
  const balances = useQueries({
    queries: (overview.data?.children ?? []).map((child) => ({
      queryKey: ['points', child.childId, 'balance'],
      queryFn: () => pointsApi.balance(child.childId),
    })),
  });

  if (overview.isPending)
    return (
      <div className="route-status" role="status">
        {messages.overview.loading}
      </div>
    );
  if (overview.isError)
    return (
      <div className="notice error" role="alert">
        {messages.overview.loadError}{' '}
        <button
          type="button"
          onClick={() => {
            void overview.refetch();
          }}
        >
          Try again
        </button>
      </div>
    );

  return (
    <section className="page-stack overview-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Parent Mode</p>
          <h1>{messages.overview.title}</h1>
          <p>{messages.overview.intro}</p>
        </div>
      </div>
      <Link className="card pending-summary" to="/parent/review">
        <span>
          <strong>{overview.data.pending} waiting for review</strong>
          <small>
            {overview.data.pending
              ? 'Review submitted work and award points.'
              : 'Everything submitted is reviewed.'}
          </small>
        </span>
        <span aria-hidden="true">→</span>
      </Link>
      <section aria-labelledby="children-progress-title">
        <h2 id="children-progress-title">Children</h2>
        {!overview.data.children.length ? (
          <div className="empty-state">
            <p>{messages.overview.empty}</p>
          </div>
        ) : (
          <div className="overview-grid">
            {overview.data.children.map((child, index) => {
              return (
                <article
                  className="card child-progress-card"
                  key={child.childId}
                >
                  <div className="child-summary">
                    <ChildAvatar avatar={child.avatar} color={child.color} />
                    <div className="child-summary-name">
                      <h3>{child.nickname}</h3>
                      <p>
                        {balances[index]?.isError
                          ? 'Points unavailable'
                          : `${balances[index]?.data?.points ?? '—'} points`}
                      </p>
                    </div>
                    <span className="count-badge">
                      {child.pending} waiting
                      <span className="sr-only"> for parent review</span>
                    </span>
                  </div>
                  <div className="progress-summary">
                    <div>
                      <strong>Today</strong>
                      <span>
                        {child.completed} of {child.total} completed
                      </span>
                    </div>
                    <progress
                      aria-label={`${child.nickname}'s completed work today`}
                      max={Math.max(child.total, 1)}
                      value={child.completed}
                    />
                  </div>
                  <Link
                    className="button button-secondary"
                    to={`/parent/reports?childId=${child.childId}`}
                  >
                    View progress
                    <span className="sr-only"> for {child.nickname}</span>
                  </Link>
                </article>
              );
            })}
          </div>
        )}
      </section>
    </section>
  );
}
