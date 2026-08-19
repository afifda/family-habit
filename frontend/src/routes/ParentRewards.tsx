import { FormEvent, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  childrenApi,
  householdApi,
  rewardsApi,
  type Reward,
  type RewardInput,
  type RewardRedemption,
} from '../api/client';
import { messageForError } from '../api/errors';
import { AccessibleDialog } from '../components/AccessibleDialog';
import { RewardEligibilityPanel } from '../components/RewardEligibilityPanel';

const empty: RewardInput = {
  title: '',
  description: '',
  icon: '🎁',
  costPoints: 25,
  availabilityScope: 'all_active_children',
  eligibleChildIds: [],
};

export function ParentRewards() {
  const cache = useQueryClient();
  const household = useQuery({
    queryKey: ['household'],
    queryFn: () => householdApi.get(),
  });
  const children = useQuery({
    queryKey: ['children', false],
    queryFn: () => childrenApi.list(false),
  });
  const rewards = useQuery({
    queryKey: ['rewards'],
    queryFn: () => rewardsApi.list(),
    enabled: household.data?.rewardsEnabled === true,
  });
  const queue = useQuery({
    queryKey: ['reward-redemptions', 'requested'],
    queryFn: () => rewardsApi.redemptions('requested'),
    enabled: household.data?.rewardsEnabled === true,
  });
  const [editing, setEditing] = useState<Reward | 'new' | null>(null);
  const [draft, setDraft] = useState<RewardInput>(empty);
  const [cancel, setCancel] = useState<RewardRedemption | null>(null);
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const createKey = useRef(crypto.randomUUID());
  const toggleKeys = useRef(new Map<boolean, string>());

  const invalidate = async () => {
    await Promise.all([
      cache.invalidateQueries({ queryKey: ['rewards'] }),
      cache.invalidateQueries({ queryKey: ['reward-redemptions'] }),
    ]);
  };
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => {
      if (!household.data)
        throw new Error('Household settings are not loaded.');
      let idempotencyKey = toggleKeys.current.get(enabled);
      if (!idempotencyKey) {
        idempotencyKey = crypto.randomUUID();
        toggleKeys.current.set(enabled, idempotencyKey);
      }
      return householdApi.update(
        { rewardsEnabled: enabled },
        { version: household.data.version, idempotencyKey },
      );
    },
    onSuccess: (data, enabled) => {
      toggleKeys.current.delete(enabled);
      cache.setQueryData(['household'], data);
    },
    onError: (cause) => setError(messageForError(cause)),
  });
  const save = useMutation({
    mutationFn: () =>
      editing === 'new'
        ? rewardsApi.create(draft, createKey.current)
        : rewardsApi.update(editing!.id, draft, editing!.version),
    onSuccess: async () => {
      createKey.current = crypto.randomUUID();
      setEditing(null);
      await invalidate();
    },
    onError: (cause) => setError(messageForError(cause)),
  });
  const archive = useMutation({
    mutationFn: (reward: Reward) =>
      rewardsApi.archive(reward.id, reward.version),
    onSuccess: invalidate,
    onError: (cause) => setError(messageForError(cause)),
  });
  const fulfill = useMutation({
    mutationFn: (redemption: RewardRedemption) =>
      rewardsApi.fulfill(redemption),
    onSuccess: invalidate,
    onError: (cause) => setError(messageForError(cause)),
  });
  const cancelMutation = useMutation({
    mutationFn: () => rewardsApi.cancel(cancel!, reason.trim()),
    onSuccess: async () => {
      setCancel(null);
      await invalidate();
    },
    onError: (cause) => setError(messageForError(cause)),
  });

  function open(reward?: Reward) {
    setError('');
    setDraft(
      reward
        ? {
            title: reward.title,
            description: reward.description ?? '',
            icon: reward.icon ?? '',
            costPoints: reward.costPoints,
            availabilityScope: reward.availabilityScope,
            eligibleChildIds: reward.eligibleChildIds,
          }
        : { ...empty },
    );
    setEditing(reward ?? 'new');
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (!draft.title.trim()) return setError('Enter a reward title.');
    if (
      !Number.isInteger(draft.costPoints) ||
      draft.costPoints < 1 ||
      draft.costPoints > 10_000
    )
      return setError('Cost must be a whole number from 1 to 10,000.');
    if (
      draft.availabilityScope === 'selected_children' &&
      !draft.eligibleChildIds?.length
    )
      return setError('Choose at least one eligible child.');
    save.mutate();
  }
  const loadError =
    household.error ?? children.error ?? rewards.error ?? queue.error;
  if (household.isPending || children.isPending)
    return <p role="status">Loading rewards…</p>;

  return (
    <section className="page-stack" aria-labelledby="rewards-title">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Parent Mode</p>
          <h1 id="rewards-title">Rewards</h1>
          <p className="page-intro">
            Kids reserve points when they request a reward. Cancelling returns
            the exact amount.
          </p>
        </div>
      </div>
      {loadError && (
        <div className="notice error" role="alert">
          Rewards did not load.{' '}
          <button
            type="button"
            onClick={() => {
              void household.refetch();
              void rewards.refetch();
              void queue.refetch();
            }}
          >
            Try again
          </button>
        </div>
      )}
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      <div className="card reward-toggle">
        <div>
          <h2>Point redemption</h2>
          <p className="muted">
            Optional and off by default. Turning it off hides rewards but
            preserves the catalog and history.
          </p>
        </div>
        <label className="toggle-row">
          <input
            type="checkbox"
            checked={household.data?.rewardsEnabled ?? false}
            disabled={toggle.isPending}
            onChange={(e) => toggle.mutate(e.target.checked)}
          />{' '}
          Enable rewards
        </label>
      </div>
      {household.data?.rewardsEnabled && (
        <>
          <RewardEligibilityPanel />
          <section aria-labelledby="requests-title">
            <div className="section-heading">
              <div>
                <h2 id="requests-title">Requests</h2>
                <p className="muted">Points are already reserved.</p>
              </div>
              <span className="count-badge">{queue.data?.length ?? 0}</span>
            </div>
            {queue.isPending ? (
              <p role="status">Loading requests…</p>
            ) : (queue.data?.length ?? 0) === 0 ? (
              <div className="empty-state">
                <p>No reward requests are waiting.</p>
              </div>
            ) : (
              <ul className="reward-grid">
                {queue.data?.map((item) => (
                  <li className="card reward-card" key={item.id}>
                    <div>
                      <span className="eyebrow">
                        {children.data?.find(({ id }) => id === item.childId)
                          ?.nickname ?? 'Child'}
                      </span>
                      <h3>{item.rewardTitle}</h3>
                      <p>{item.costPoints} points reserved</p>
                    </div>
                    <div className="card-actions">
                      <button
                        className="button button-primary"
                        disabled={fulfill.isPending}
                        onClick={() => fulfill.mutate(item)}
                      >
                        Mark fulfilled
                      </button>
                      <button
                        className="button button-secondary"
                        onClick={() => {
                          setReason('');
                          setCancel(item);
                        }}
                      >
                        Cancel & refund
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section aria-labelledby="catalog-title">
            <div className="section-heading">
              <div>
                <h2 id="catalog-title">Reward catalog</h2>
                <p className="muted">
                  All active children are selected by default.
                </p>
              </div>
              <button className="button button-primary" onClick={() => open()}>
                New reward
              </button>
            </div>
            {rewards.isPending ? (
              <p role="status">Loading catalog…</p>
            ) : (rewards.data?.length ?? 0) === 0 ? (
              <div className="empty-state">
                <p>
                  No rewards yet. Add something meaningful your kids can choose.
                </p>
              </div>
            ) : (
              <ul className="reward-grid">
                {rewards.data?.map((reward) => (
                  <li className="card reward-card" key={reward.id}>
                    <span className="reward-icon" aria-hidden="true">
                      {reward.icon || '🎁'}
                    </span>
                    <div>
                      <h3>{reward.title}</h3>
                      <p>
                        {reward.costPoints} points ·{' '}
                        {reward.availabilityScope === 'all_active_children'
                          ? 'All active children'
                          : `${reward.eligibleChildIds.length} eligible`}
                      </p>
                      {reward.description && (
                        <p className="muted">{reward.description}</p>
                      )}
                    </div>
                    <div className="card-actions">
                      <button
                        className="button button-secondary"
                        onClick={() => open(reward)}
                      >
                        Edit
                      </button>
                      <button
                        className="button button-danger"
                        disabled={archive.isPending}
                        onClick={() => archive.mutate(reward)}
                      >
                        Archive
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
          {editing && (
            <AccessibleDialog
              titleId="reward-editor-heading"
              backdropClassName="work-editor-backdrop"
              className="work-editor-dialog"
              onClose={() => setEditing(null)}
            >
              <div className="work-editor-header">
                <div>
                  <p className="eyebrow">Reward</p>
                  <h2 id="reward-editor-heading">
                    {editing === 'new'
                      ? 'Create reward'
                      : `Edit ${editing.title}`}
                  </h2>
                </div>
                <button
                  className="text-button"
                  type="button"
                  onClick={() => setEditing(null)}
                >
                  Close
                </button>
              </div>
              <div className="work-editor-body">
                <form className="auth-form" onSubmit={submit}>
                  {error && (
                    <div className="form-alert" role="alert">
                      {error}
                    </div>
                  )}
                  <label className="form-field">
                    Title
                    <input
                      data-initial-focus
                      required
                      maxLength={100}
                      value={draft.title}
                      onChange={(e) =>
                        setDraft({ ...draft, title: e.target.value })
                      }
                    />
                  </label>
                  <label className="form-field">
                    Description (optional)
                    <textarea
                      maxLength={500}
                      value={draft.description ?? ''}
                      onChange={(e) =>
                        setDraft({ ...draft, description: e.target.value })
                      }
                    />
                  </label>
                  <div className="form-row">
                    <label className="form-field">
                      Icon
                      <input
                        maxLength={40}
                        value={draft.icon ?? ''}
                        onChange={(e) =>
                          setDraft({ ...draft, icon: e.target.value })
                        }
                      />
                    </label>
                    <label className="form-field">
                      Cost in points
                      <input
                        type="number"
                        min="1"
                        step="1"
                        value={draft.costPoints}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            costPoints: Number(e.target.value),
                          })
                        }
                      />
                    </label>
                  </div>
                  <fieldset>
                    <legend>Eligible children</legend>
                    <label className="toggle-row">
                      <input
                        type="radio"
                        name="availability"
                        checked={
                          draft.availabilityScope === 'all_active_children'
                        }
                        onChange={() =>
                          setDraft({
                            ...draft,
                            availabilityScope: 'all_active_children',
                            eligibleChildIds: [],
                          })
                        }
                      />
                      All active children
                    </label>
                    <label className="toggle-row">
                      <input
                        type="radio"
                        name="availability"
                        checked={draft.availabilityScope === 'selected_children'}
                        onChange={() =>
                          setDraft({
                            ...draft,
                            availabilityScope: 'selected_children',
                          })
                        }
                      />
                      Selected children
                    </label>
                    {children.data?.map((child) => (
                      <label className="toggle-row" key={child.id}>
                        <input
                          type="checkbox"
                          disabled={
                            draft.availabilityScope !== 'selected_children'
                          }
                          checked={
                            draft.eligibleChildIds?.includes(child.id) ?? false
                          }
                          onChange={(e) =>
                            setDraft({
                              ...draft,
                              eligibleChildIds: e.target.checked
                                ? [...(draft.eligibleChildIds ?? []), child.id]
                                : (draft.eligibleChildIds ?? []).filter(
                                    (id) => id !== child.id,
                                  ),
                            })
                          }
                        />
                        {child.nickname}
                      </label>
                    ))}
                  </fieldset>
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
                      {save.isPending ? 'Saving…' : 'Save reward'}
                    </button>
                  </div>
                </form>
              </div>
            </AccessibleDialog>
          )}
        </>
      )}
      {cancel && (
        <AccessibleDialog
          titleId="cancel-redemption-title"
          onClose={() => setCancel(null)}
        >
          <h2 id="cancel-redemption-title">Cancel {cancel.rewardTitle}?</h2>
          <p>
            <strong>+{cancel.costPoints} points</strong> will be returned
            immediately.
          </p>
          <label className="form-field">
            Reason
            <textarea
              autoFocus
              required
              maxLength={500}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </label>
          {cancelMutation.isError && (
            <div className="notice error" role="alert">
              {messageForError(cancelMutation.error)}
            </div>
          )}
          <div className="form-actions">
            <button
              className="button button-danger"
              disabled={!reason.trim() || cancelMutation.isPending}
              onClick={() => cancelMutation.mutate()}
            >
              {cancelMutation.isPending
                ? 'Cancelling…'
                : `Cancel & refund ${cancel.costPoints}`}
            </button>
            <button
              className="button button-secondary"
              onClick={() => setCancel(null)}
            >
              Keep request
            </button>
          </div>
        </AccessibleDialog>
      )}
    </section>
  );
}
