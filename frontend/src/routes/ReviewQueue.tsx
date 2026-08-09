import { useEffect, useRef, useState } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import { ReviewItem, childrenApi, reviewApi } from '../api/client';
import { ApiError, messageForError } from '../api/errors';
import { AccessibleDialog } from '../components/AccessibleDialog';

type Decision = { item: ReviewItem; kind: 'reject' };

function submittedLabel(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value));
}

export function ReviewQueue() {
  const queryClient = useQueryClient();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [childFilter, setChildFilter] = useState('');
  const [decision, setDecision] = useState<Decision | null>(null);
  const [reason, setReason] = useState('');
  const [feedback, setFeedback] = useState('');
  const [mutationError, setMutationError] = useState('');
  const [lastDecidedId, setLastDecidedId] = useState('');
  const keys = useRef(new Map<string, string>());

  const childrenQuery = useQuery({
    queryKey: ['children', 'review-filter'],
    queryFn: () => childrenApi.list(true),
  });
  const pending = useInfiniteQuery({
    queryKey: ['review', 'pending', childFilter],
    queryFn: ({ pageParam }) =>
      reviewApi.pending(childFilter || undefined, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.page.nextCursor || undefined,
  });
  const items = pending.data?.pages.flatMap((page) => page.data) ?? [];

  const decide = useMutation({
    mutationFn: async ({
      item,
      kind,
    }: {
      item: ReviewItem;
      kind: 'approve' | 'reject';
    }) => {
      const operation = `${kind}:${item.id}`;
      let key = keys.current.get(operation);
      if (!key) {
        key = crypto.randomUUID();
        keys.current.set(operation, key);
      }
      if (kind === 'approve')
        return reviewApi.approve(item.id, item.occurrenceVersion, key);
      return reviewApi.reject(
        item.id,
        item.occurrenceVersion,
        reason.trim(),
        key,
      );
    },
    onSuccess: (_, variables) => {
      keys.current.delete(`${variables.kind}:${variables.item.id}`);
      setDecision(null);
      setReason('');
      setMutationError('');
      setLastDecidedId(variables.item.id);
      setFeedback(
        variables.kind === 'approve'
          ? `Approved. ${variables.item.points} points added to ${variables.item.childName}.`
          : `${variables.item.title} is ready for another try.`,
      );
      void queryClient.invalidateQueries({ queryKey: ['review'] });
      void queryClient.invalidateQueries({ queryKey: ['points'] });
    },
    onError: (error, variables) => {
      if (
        error instanceof ApiError &&
        ['version_conflict', 'invalid_state_transition'].includes(error.code)
      ) {
        keys.current.delete(`${variables.kind}:${variables.item.id}`);
        setMutationError(
          'This item changed, so the review queue was refreshed.',
        );
        void pending.refetch();
        return;
      }
      setMutationError(messageForError(error));
    },
  });

  useEffect(() => {
    if (!feedback) return;
    const frame = requestAnimationFrame(() => {
      const next = document.querySelector<HTMLElement>(
        `.review-card:not([data-review-id="${lastDecidedId}"]) button`,
      );
      (next ?? headingRef.current)?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [feedback, items.length, lastDecidedId]);

  const children =
    childrenQuery.data?.map((child) => [child.id, child.nickname] as const) ??
    [];

  return (
    <section className="page-stack review-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Parent review</p>
          <h1 ref={headingRef} tabIndex={-1}>
            Review completions
          </h1>
          <p>Celebrate finished work or send it back with a kind note.</p>
        </div>
        {pending.data && (
          <span className="count-badge">{items.length} waiting</span>
        )}
      </div>

      {feedback && (
        <div className="notice success" role="status">
          {feedback}
        </div>
      )}
      {mutationError && (
        <div className="notice error" role="alert">
          {mutationError} The same decision can be safely tried again.
        </div>
      )}

      {children.length > 1 && (
        <div className="filter-row" aria-label="Filter review queue">
          <button
            className="button button-secondary"
            aria-pressed={!childFilter}
            onClick={() => setChildFilter('')}
            type="button"
          >
            All
          </button>
          {children.map(([id, name]) => (
            <button
              className="button button-secondary"
              aria-pressed={childFilter === id}
              onClick={() => setChildFilter(id)}
              type="button"
              key={id}
            >
              {name}
            </button>
          ))}
        </div>
      )}

      {pending.isPending && (
        <div className="route-status" role="status">
          Loading work to review…
        </div>
      )}
      {pending.isError && (
        <div className="notice error" role="alert">
          We could not load the review queue.{' '}
          <button type="button" onClick={() => void pending.refetch()}>
            Try again
          </button>
        </div>
      )}
      {items.length === 0 && pending.isSuccess && (
        <div className="empty-state">
          <h2>Everything is reviewed</h2>
          <p>Newly submitted work will appear here.</p>
        </div>
      )}
      <div className="review-grid">
        {items.map((item) => {
          const itemBusy =
            decide.isPending && decide.variables.item.id === item.id;
          return (
            <article
              className="card review-card"
              key={item.id}
              data-review-id={item.id}
              aria-busy={itemBusy}
            >
              <p className="eyebrow">{item.childName}</p>
              <h2>{item.title}</h2>
              <p className="muted">
                Sent {submittedLabel(item.submittedAt)} · {item.points}{' '}
                {item.points === 1 ? 'point' : 'points'}
              </p>
              <div className="form-actions">
                <button
                  className="button button-secondary"
                  type="button"
                  disabled={itemBusy}
                  onClick={() => {
                    setDecision({ item, kind: 'reject' });
                    setMutationError('');
                  }}
                >
                  Not yet
                </button>
                <button
                  className="button button-primary"
                  type="button"
                  disabled={itemBusy}
                  onClick={() => decide.mutate({ item, kind: 'approve' })}
                >
                  Approve · +{item.points}
                </button>
              </div>
            </article>
          );
        })}
      </div>
      {pending.hasNextPage && (
        <button
          className="button button-secondary"
          type="button"
          disabled={pending.isFetchingNextPage}
          onClick={() => void pending.fetchNextPage()}
        >
          {pending.isFetchingNextPage
            ? 'Loading more…'
            : 'Load more submissions'}
        </button>
      )}

      {decision && (
        <AccessibleDialog
          titleId="reject-title"
          onClose={() => {
            setDecision(null);
            setReason('');
          }}
        >
          <h2 id="reject-title">Ready for another try?</h2>
          <p>
            {decision.item.title} will return to {decision.item.childName}
            ’s To do list. No points will be removed.
          </p>
          <label className="form-field">
            Kind note (optional)
            <textarea
              data-initial-focus
              maxLength={500}
              value={reason}
              onChange={(event) => {
                keys.current.delete(`reject:${decision.item.id}`);
                setReason(event.target.value);
              }}
            />
          </label>
          <div className="form-actions">
            <button
              className="button button-secondary"
              type="button"
              onClick={() => {
                setDecision(null);
                setReason('');
              }}
            >
              Keep waiting
            </button>
            <button
              className="button button-primary"
              type="button"
              disabled={decide.isPending}
              onClick={() =>
                decide.mutate({ item: decision.item, kind: 'reject' })
              }
            >
              {decide.isPending ? 'Saving…' : 'Return to To do'}
            </button>
          </div>
        </AccessibleDialog>
      )}
    </section>
  );
}
