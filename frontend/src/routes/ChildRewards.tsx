import { useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { rewardsApi, type ChildReward } from '../api/client';
import { ApiError, messageForError } from '../api/errors';
import { AccessibleDialog } from '../components/AccessibleDialog';

export function ChildRewards() {
  const cache = useQueryClient();
  const catalog = useQuery({
    queryKey: ['child-rewards'],
    queryFn: () => rewardsApi.childCatalog(),
  });
  const history = useQuery({
    queryKey: ['child-reward-redemptions'],
    queryFn: () => rewardsApi.childRedemptions(),
  });
  const [selected, setSelected] = useState<ChildReward | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [error, setError] = useState('');
  const keys = useRef(new Map<string, string>());

  const redeem = useMutation({
    mutationFn: (reward: ChildReward) => {
      let key = keys.current.get(reward.id);
      if (!key) {
        key = crypto.randomUUID();
        keys.current.set(reward.id, key);
      }
      return rewardsApi.redeem(
        reward.id,
        reward.version,
        reward.costPoints,
        key,
      );
    },
    onSuccess: async (redemption, reward) => {
      keys.current.delete(reward.id);
      setSelected(null);
      setAnnouncement(
        `${reward.title} requested. ${redemption.costPoints} points reserved.`,
      );
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['child-rewards'] }),
        cache.invalidateQueries({ queryKey: ['child-reward-redemptions'] }),
        cache.invalidateQueries({ queryKey: ['points'] }),
      ]);
    },
    onError: async (cause) => {
      setError(messageForError(cause));
      if (cause instanceof ApiError) {
        if (selected) keys.current.delete(selected.id);
        await catalog.refetch();
      }
    },
  });

  if (catalog.isPending || history.isPending)
    return (
      <p className="route-status" role="status">
        Loading rewards…
      </p>
    );
  if (catalog.isError || history.isError) {
    const forbidden =
      catalog.error instanceof ApiError &&
      [403, 404].includes(catalog.error.status);
    return (
      <section className="page-stack">
        <div className="notice error" role="alert">
          <h1>
            {forbidden ? 'Rewards are not available' : 'Rewards did not load'}
          </h1>
          <p>
            {forbidden
              ? 'Ask a parent if your family would like to use point rewards.'
              : 'Check your connection and try again. No points were changed.'}
          </p>
          {!forbidden && (
            <button
              onClick={() => {
                void catalog.refetch();
                void history.refetch();
              }}
            >
              Try again
            </button>
          )}
        </div>
      </section>
    );
  }
  const balance = catalog.data?.balance ?? 0;
  const rewards = catalog.data?.data ?? [];
  const eligibility = catalog.data?.eligibility;

  const eligibilityMessage = (() => {
    if (!eligibility?.policyEnabled) return null;
    if (eligibility.status === 'collecting') {
      return eligibility.pointsShortfall > 0
        ? `${eligibility.pointsShortfall.toLocaleString()} more approved points will reach this period’s goal.`
        : 'You reached the points goal. Keep going until this period finishes.';
    }
    if (eligibility.status === 'awaiting_evaluation')
      return 'This period is complete. Final approvals are being counted.';
    if (eligibility.status === 'eligible') {
      const limit = eligibility.maximumRedemptions;
      return limit === null
        ? 'Your collection goal is complete and rewards are unlocked.'
        : `Rewards are unlocked. ${Math.max(0, limit - eligibility.redemptionsUsed)} of ${limit} requests remaining.`;
    }
    return 'A new collection period is a fresh chance to unlock rewards.';
  })();

  return (
    <section
      className="page-stack child-rewards-page"
      aria-labelledby="child-rewards-title"
    >
      <div className="points-hero">
        <p className="eyebrow">Choose for yourself</p>
        <h1 id="child-rewards-title">Rewards</h1>
        <p>
          <strong>{balance.toLocaleString()} points</strong> available
        </p>
      </div>
      <p className="visually-hidden" aria-live="polite">
        {announcement}
      </p>
      {eligibility?.policyEnabled && (
        <section
          className={`card child-eligibility ${eligibility.status}`}
          aria-labelledby="collection-progress-title"
        >
          <div>
            <p className="eyebrow">Collection goal</p>
            <h2 id="collection-progress-title">
              {eligibility.status === 'eligible'
                ? 'Rewards unlocked'
                : eligibility.status === 'awaiting_evaluation'
                  ? 'Waiting for final approvals'
                  : eligibility.status === 'not_eligible'
                    ? 'A new chance begins now'
                    : 'Keep collecting'}
            </h2>
            <p>{eligibilityMessage}</p>
          </div>
          <dl className="eligibility-metrics">
            <div>
              <dt>Approved points</dt>
              <dd>
                {eligibility.pointsCollected.toLocaleString()} /{' '}
                {eligibility.minimumPoints.toLocaleString()}
              </dd>
            </div>
            {eligibility.minimumCompletionPercentage !== null && (
              <div>
                <dt>Activities completed</dt>
                <dd>
                  {eligibility.completionPercentage ?? 0}% /{' '}
                  {eligibility.minimumCompletionPercentage}%
                </dd>
              </div>
            )}
          </dl>
          {eligibility.status === 'collecting' && (
            <progress
              max={Math.max(eligibility.minimumPoints, 1)}
              value={Math.min(
                eligibility.pointsCollected,
                eligibility.minimumPoints,
              )}
            >
              {eligibility.pointsCollected} of {eligibility.minimumPoints}
            </progress>
          )}
        </section>
      )}
      {error && (
        <div className="notice error" role="alert">
          {error} Your balance has been refreshed.
        </div>
      )}
      {rewards.length === 0 ? (
        <div className="empty-state">
          <div>
            <h2>No rewards right now</h2>
            <p>Your family has not added any rewards for you yet.</p>
          </div>
        </div>
      ) : (
        <ul className="reward-grid">
          {rewards.map((reward) => (
            <li className="card reward-card" key={reward.id}>
              <span className="reward-icon" aria-hidden="true">
                {reward.icon || '🎁'}
              </span>
              <div>
                <h2>{reward.title}</h2>
                {reward.description && <p>{reward.description}</p>}
                <strong>{reward.costPoints.toLocaleString()} points</strong>
                {!reward.canRedeem && (
                  <p className="muted">
                    {eligibility?.policyEnabled && !eligibility.canRedeem
                      ? eligibility.status === 'eligible'
                        ? 'The request limit for this unlock period has been reached.'
                        : 'Complete the collection goal to unlock this reward.'
                      : `You need ${(
                          reward.shortfallPoints ?? reward.costPoints - balance
                        ).toLocaleString()} more points.`}
                  </p>
                )}
              </div>
              <button
                className="button button-primary"
                disabled={!reward.canRedeem}
                onClick={() => {
                  setError('');
                  setSelected(reward);
                }}
              >
                Choose reward
              </button>
            </li>
          ))}
        </ul>
      )}
      <section aria-labelledby="reward-history-title">
        <h2 id="reward-history-title">My requests</h2>
        {history.data?.length === 0 ? (
          <p className="muted">Your reward requests will appear here.</p>
        ) : (
          <ol className="activity-list">
            {history.data?.map((item) => (
              <li className="card activity-row" key={item.id}>
                <div>
                  <strong>{item.rewardTitle}</strong>
                  <p className="muted">
                    {item.state === 'requested'
                      ? 'Waiting for a parent'
                      : item.state === 'fulfilled'
                        ? 'Fulfilled'
                        : 'Cancelled and refunded'}
                  </p>
                </div>
                <strong>
                  {item.state === 'cancelled'
                    ? `+${item.costPoints}`
                    : `−${item.costPoints}`}{' '}
                  points
                </strong>
              </li>
            ))}
          </ol>
        )}
      </section>
      {selected && (
        <AccessibleDialog
          titleId="redeem-title"
          onClose={() => !redeem.isPending && setSelected(null)}
        >
          <h2 id="redeem-title">Request {selected.title}?</h2>
          <dl className="point-confirmation">
            <div>
              <dt>Current balance</dt>
              <dd>{balance.toLocaleString()}</dd>
            </div>
            <div>
              <dt>Points reserved now</dt>
              <dd>−{selected.costPoints.toLocaleString()}</dd>
            </div>
            <div>
              <dt>Balance after request</dt>
              <dd>{(balance - selected.costPoints).toLocaleString()}</dd>
            </div>
          </dl>
          <p>
            A parent will fulfill it later. If they cancel, all{' '}
            {selected.costPoints} points come back.
          </p>
          {redeem.isError && (
            <div className="notice error" role="alert">
              {messageForError(redeem.error)} Try again uses the same request
              safely.
            </div>
          )}
          <div className="card-actions">
            <button
              data-initial-focus
              className="button button-primary"
              disabled={!selected.canRedeem || redeem.isPending}
              aria-busy={redeem.isPending}
              onClick={() => redeem.mutate(selected)}
            >
              {redeem.isPending
                ? 'Requesting…'
                : `Confirm −${selected.costPoints} points`}
            </button>
            <button
              className="button button-secondary"
              disabled={redeem.isPending}
              onClick={() => setSelected(null)}
            >
              Not now
            </button>
          </div>
        </AccessibleDialog>
      )}
    </section>
  );
}
