import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { pointsApi } from '../api/client';
import { useAuth } from '../auth/AuthProvider';

const activityCopy = {
  award: 'Finished work',
  approval_reversal: 'Points updated by a parent',
  manual_correction: 'Bonus from a parent',
} as const;

export function ChildPoints() {
  const { session } = useAuth();
  const childId = session?.actor === 'child' ? session.childId : undefined;
  const balance = useQuery({
    queryKey: ['points', childId, 'balance'],
    queryFn: () => pointsApi.balance(childId!),
    enabled: Boolean(childId),
  });
  const ledger = useInfiniteQuery({
    queryKey: ['points', childId, 'ledger'],
    queryFn: ({ pageParam }) => pointsApi.ledger(childId!, pageParam),
    enabled: Boolean(childId),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.page.nextCursor || undefined,
  });
  const entries = ledger.data?.pages.flatMap((page) => page.data) ?? [];

  if (balance.isPending || ledger.isPending)
    return (
      <div className="route-status" role="status">
        Loading your points…
      </div>
    );
  if (balance.isError || ledger.isError)
    return (
      <div className="notice error" role="alert">
        We could not load your points.{' '}
        <button
          type="button"
          onClick={() => {
            void balance.refetch();
            void ledger.refetch();
          }}
        >
          Try again
        </button>
      </div>
    );

  return (
    <section className="page-stack child-points-page">
      <div className="points-hero">
        <p className="eyebrow">My points</p>
        <h1>{balance.data?.points ?? 0}</h1>
        <p>Every point here came from work a parent approved.</p>
      </div>
      <section aria-labelledby="activity-title">
        <h2 id="activity-title">Recent activity</h2>
        {entries.length === 0 ? (
          <div className="empty-state">
            <p>Your first approved activity will show up here.</p>
          </div>
        ) : (
          <ol className="activity-list">
            {entries.map((entry) => (
              <li className="card activity-row" key={entry.id}>
                <div>
                  <strong>{activityCopy[entry.kind]}</strong>
                  <p className="muted">
                    {new Intl.DateTimeFormat(undefined, {
                      dateStyle: 'medium',
                    }).format(new Date(entry.createdAt))}
                  </p>
                </div>
                <strong className={entry.amount > 0 ? 'points-positive' : ''}>
                  {entry.amount > 0 ? '+' : ''}
                  {entry.amount} points
                </strong>
              </li>
            ))}
          </ol>
        )}
        {ledger.hasNextPage && (
          <button
            className="button button-secondary"
            type="button"
            disabled={ledger.isFetchingNextPage}
            onClick={() => void ledger.fetchNextPage()}
          >
            {ledger.isFetchingNextPage ? 'Loading more…' : 'Load more activity'}
          </button>
        )}
      </section>
    </section>
  );
}
