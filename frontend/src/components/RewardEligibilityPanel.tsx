import { FormEvent, useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  rewardEligibilityApi,
  type RewardEligibilityPolicy,
  type RewardEligibilityProgress,
  type RewardEligibilityStatus,
} from '../api/client';
import { messageForError } from '../api/errors';

const initialPolicy: RewardEligibilityPolicy = {
  enabled: false,
  period: 'weekly',
  minimumPoints: 100,
  minimumCompletionPercentage: null,
  maximumRedemptions: null,
  graceHours: 24,
  effectiveFrom: null,
  version: 0,
};

const statusCopy: Record<RewardEligibilityStatus, string> = {
  collecting: 'Collecting points',
  awaiting_evaluation: 'Waiting for final approvals',
  eligible: 'Rewards unlocked',
  not_eligible: 'New chance this period',
};

function formatDate(value: string | null) {
  if (!value) return 'Not available';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(
    new Date(`${value}T00:00:00`),
  );
}

function ProgressCard({ item }: { item: RewardEligibilityProgress }) {
  return (
    <li className="card eligibility-card">
      <div className="section-heading">
        <div>
          <h3>{item.childName}</h3>
          <p className="muted">
            {formatDate(item.collectionPeriodStart)}–
            {formatDate(item.collectionPeriodEnd)}
          </p>
        </div>
        <span className="status-pill">{statusCopy[item.status]}</span>
      </div>
      <dl className="eligibility-metrics">
        <div>
          <dt>Points collected</dt>
          <dd>{item.pointsCollected.toLocaleString()}</dd>
        </div>
        <div>
          <dt>Approved activities</dt>
          <dd>
            {item.approvedCount} of {item.assignedCount}
          </dd>
        </div>
        {item.completionPercentage !== null && (
          <div>
            <dt>Completion</dt>
            <dd>{item.completionPercentage}%</dd>
          </div>
        )}
      </dl>
      <ul
        className="rule-results"
        aria-label={`${item.childName} rule results`}
      >
        {item.rules.map((rule) => (
          <li key={rule.type}>
            <span aria-hidden="true">{rule.passed ? '✓' : '○'}</span>{' '}
            {rule.type === 'minimum_points'
              ? `${rule.actual} of ${rule.target} points`
              : `${rule.actual}% of ${rule.target}% completion`}
            <span className="visually-hidden">
              {rule.passed ? ' passed' : ' not reached yet'}
            </span>
          </li>
        ))}
      </ul>
    </li>
  );
}

export function RewardEligibilityPanel() {
  const cache = useQueryClient();
  const policy = useQuery({
    queryKey: ['reward-eligibility-policy'],
    queryFn: () => rewardEligibilityApi.policy(),
  });
  const progress = useQuery({
    queryKey: ['reward-eligibility-progress'],
    queryFn: () => rewardEligibilityApi.progress(),
  });
  const evaluations = useQuery({
    queryKey: ['reward-eligibility-evaluations'],
    queryFn: () => rewardEligibilityApi.evaluations(),
  });
  const [draft, setDraft] = useState(initialPolicy);
  const [error, setError] = useState('');
  const saveKey = useRef(crypto.randomUUID());

  useEffect(() => {
    if (policy.data)
      setDraft({
        ...policy.data,
        graceHours: policy.data.period === 'daily' ? 0 : policy.data.graceHours,
      });
  }, [policy.data]);

  const save = useMutation({
    mutationFn: () =>
      rewardEligibilityApi.updatePolicy(
        {
          enabled: draft.enabled,
          period: draft.period,
          minimumPoints: draft.minimumPoints,
          minimumCompletionPercentage: draft.minimumCompletionPercentage,
          maximumRedemptions: draft.maximumRedemptions,
          graceHours: draft.graceHours,
        },
        policy.data?.version ?? 0,
        saveKey.current,
      ),
    onSuccess: async (saved) => {
      saveKey.current = crypto.randomUUID();
      setError('');
      cache.setQueryData(['reward-eligibility-policy'], saved);
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['reward-eligibility-progress'] }),
        cache.invalidateQueries({
          queryKey: ['reward-eligibility-evaluations'],
        }),
      ]);
    },
    onError: (cause) => setError(messageForError(cause)),
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (
      !Number.isInteger(draft.minimumPoints) ||
      draft.minimumPoints < 1 ||
      draft.minimumPoints > 1_000_000
    ) {
      setError('Minimum points must be a whole number from 1 to 1,000,000.');
      return;
    }
    save.mutate();
  }

  if (policy.isPending) return <p role="status">Loading reward rules…</p>;

  return (
    <section className="eligibility-panel" aria-labelledby="eligibility-title">
      <div className="section-heading">
        <div>
          <h2 id="eligibility-title">Collection period rules</h2>
          <p className="muted">
            Optionally unlock redemptions after a child reaches the goals for a
            completed period. Their point balance never resets.
          </p>
        </div>
      </div>
      {(policy.isError || error) && (
        <div className="notice error" role="alert">
          {error || 'Reward rules did not load.'}
        </div>
      )}
      <form className="card settings-form" onSubmit={submit}>
        <label className="toggle-row">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(event) =>
              setDraft({ ...draft, enabled: event.target.checked })
            }
          />
          Require a completed collection goal before redeeming
        </label>
        <p className="muted">
          Off by default. Reward cost and current balance checks always still
          apply.
        </p>
        <fieldset disabled={!draft.enabled || save.isPending}>
          <legend>Eligibility goal</legend>
          <div className="form-row">
            <label className="form-field">
              Collection period
              <select
                value={draft.period}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    period: event.target
                      .value as RewardEligibilityPolicy['period'],
                    graceHours:
                      event.target.value === 'daily' ? 0 : draft.graceHours,
                  })
                }
              >
                <option value="daily">Daily</option>
                <option value="weekly">Weekly</option>
                <option value="monthly">Monthly</option>
              </select>
            </label>
            <label className="form-field">
              Minimum approved points
              <input
                type="number"
                min="1"
                max="1000000"
                step="1"
                value={draft.minimumPoints}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    minimumPoints: Number(event.target.value),
                  })
                }
              />
            </label>
          </div>
          <div className="form-row">
            <label className="form-field">
              Minimum completion % (optional)
              <input
                type="number"
                min="1"
                max="100"
                step="1"
                value={draft.minimumCompletionPercentage ?? ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    minimumCompletionPercentage: event.target.value
                      ? Number(event.target.value)
                      : null,
                  })
                }
              />
            </label>
            <label className="form-field">
              Maximum redemptions (optional)
              <input
                type="number"
                min="1"
                max="100"
                step="1"
                value={draft.maximumRedemptions ?? ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    maximumRedemptions: event.target.value
                      ? Number(event.target.value)
                      : null,
                  })
                }
              />
            </label>
          </div>
          <label className="form-field">
            Time for final approvals
            <select
              value={draft.graceHours}
              disabled={draft.period === 'daily'}
              aria-describedby={
                draft.period === 'daily' ? 'daily-grace-help' : undefined
              }
              onChange={(event) =>
                setDraft({
                  ...draft,
                  graceHours: Number(
                    event.target.value,
                  ) as RewardEligibilityPolicy['graceHours'],
                })
              }
            >
              <option value="0">No extra time</option>
              <option value="12">12 hours</option>
              <option value="24">24 hours (recommended)</option>
              <option value="48">48 hours</option>
            </select>
            {draft.period === 'daily' && (
              <span id="daily-grace-help" className="muted">
                Daily periods are evaluated without extra approval time.
              </span>
            )}
          </label>
        </fieldset>
        <p className="notice info">
          Changes take effect from the next {draft.period} collection period.
          The current period will keep its existing rules.
        </p>
        {policy.data?.effectiveFrom && (
          <p className="muted">
            Scheduled from {formatDate(policy.data.effectiveFrom)}.
          </p>
        )}
        <button className="button button-primary" disabled={save.isPending}>
          {save.isPending ? 'Saving rules…' : 'Save collection rules'}
        </button>
      </form>

      {(draft.enabled ||
        progress.data?.some((item) => item.policyEnabled === true)) && (
        <>
          <section aria-labelledby="current-progress-title">
            <h3 id="current-progress-title">Current progress</h3>
            {progress.isPending ? (
              <p role="status">Loading current progress…</p>
            ) : progress.isError ? (
              <div className="notice error" role="alert">
                Current progress did not load.
              </div>
            ) : progress.data?.length ? (
              <ul className="eligibility-grid">
                {progress.data.map((item) => (
                  <ProgressCard key={item.childId} item={item} />
                ))}
              </ul>
            ) : (
              <p className="muted">No child progress is available yet.</p>
            )}
          </section>
          <section aria-labelledby="evaluation-history-title">
            <h3 id="evaluation-history-title">Previous periods</h3>
            {evaluations.isPending ? (
              <p role="status">Loading previous periods…</p>
            ) : evaluations.isError ? (
              <div className="notice error" role="alert">
                Previous periods did not load.
              </div>
            ) : evaluations.data?.length ? (
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th scope="col">Child</th>
                      <th scope="col">Period ended</th>
                      <th scope="col">Result</th>
                      <th scope="col">Points</th>
                    </tr>
                  </thead>
                  <tbody>
                    {evaluations.data.map((item) => (
                      <tr key={item.id}>
                        <th scope="row">{item.childName}</th>
                        <td>{formatDate(item.collectionPeriodEnd)}</td>
                        <td>{statusCopy[item.status]}</td>
                        <td>{item.pointsCollected.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="muted">Completed periods will appear here.</p>
            )}
          </section>
        </>
      )}
    </section>
  );
}
