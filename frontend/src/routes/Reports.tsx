import { FormEvent, useMemo, useRef, useState } from 'react';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';

import {
  ChildReport,
  HistoryOccurrence,
  ReportPeriod,
  childrenApi,
  householdApi,
  pointsApi,
  reportsApi,
  reviewApi,
} from '../api/client';
import { messageForError } from '../api/errors';
import { AccessibleDialog } from '../components/AccessibleDialog';

function todayIn(timezone: string) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts();
  const get = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value;
  return `${get('year')}-${get('month')}-${get('day')}`;
}

function shiftAnchor(value: string, period: ReportPeriod, direction: -1 | 1) {
  const date = new Date(`${value}T12:00:00Z`);
  if (period === 'day') date.setUTCDate(date.getUTCDate() + direction);
  if (period === 'week') date.setUTCDate(date.getUTCDate() + direction * 7);
  if (period === 'month') date.setUTCMonth(date.getUTCMonth() + direction);
  return date.toISOString().slice(0, 10);
}

function periodLabel(report: ChildReport) {
  const format = (value: string) =>
    new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeZone: 'UTC',
    }).format(new Date(`${value}T12:00:00Z`));
  return report.startDate === report.endDate
    ? format(report.startDate)
    : `${format(report.startDate)} – ${format(report.endDate)}`;
}

function statusLabel(item: HistoryOccurrence) {
  return (
    {
      not_started: 'Still to do',
      pending_approval: 'Waiting for parent',
      approved: 'Approved',
      approval_reversed: 'Approval updated',
      cancelled: 'Cancelled',
    } as const
  )[item.status];
}

export function Reports() {
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const mutationKey = useRef('');
  const [childId, setChildId] = useState('');
  const [period, setPeriod] = useState<ReportPeriod>('week');
  const [anchor, setAnchor] = useState('');
  const [correctionOpen, setCorrectionOpen] = useState(false);
  const [correctionConfirm, setCorrectionConfirm] = useState(false);
  const [correctionPoints, setCorrectionPoints] = useState('');
  const [reason, setReason] = useState('');
  const [reverseItem, setReverseItem] = useState<HistoryOccurrence | null>(
    null,
  );
  const [feedback, setFeedback] = useState('');

  const children = useQuery({
    queryKey: ['children'],
    queryFn: () => childrenApi.list(),
  });
  const household = useQuery({
    queryKey: ['household'],
    queryFn: () => householdApi.get(),
  });
  const linkedChildId = searchParams.get('childId') ?? '';
  const selectedId = childId || linkedChildId || children.data?.[0]?.id || '';
  const activeAnchor =
    anchor || (household.data ? todayIn(household.data.timezone) : '');
  const selectedChild = children.data?.find((child) => child.id === selectedId);
  const report = useQuery({
    queryKey: ['reports', selectedId, period, activeAnchor],
    queryFn: () => reportsApi.child(selectedId, period, activeAnchor),
    enabled: Boolean(selectedId && activeAnchor),
  });
  const balance = useQuery({
    queryKey: ['points', selectedId, 'balance'],
    queryFn: () => pointsApi.balance(selectedId),
    enabled: Boolean(selectedId),
  });
  const history = useInfiniteQuery({
    queryKey: ['points', selectedId, 'history'],
    queryFn: ({ pageParam }) =>
      pointsApi.history(selectedId, undefined, undefined, pageParam),
    enabled: Boolean(selectedId),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.page.nextCursor || undefined,
  });
  const historyItems = history.data?.pages.flatMap((page) => page.data) ?? [];
  const ledger = useInfiniteQuery({
    queryKey: ['points', selectedId, 'ledger', 'parent'],
    queryFn: ({ pageParam }) => pointsApi.ledger(selectedId, pageParam),
    enabled: Boolean(selectedId),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.page.nextCursor || undefined,
  });
  const ledgerItems = ledger.data?.pages.flatMap((page) => page.data) ?? [];

  const correction = useMutation({
    mutationFn: () => {
      if (!mutationKey.current) mutationKey.current = crypto.randomUUID();
      return pointsApi.correct(
        selectedId,
        Number(correctionPoints),
        reason.trim(),
        mutationKey.current,
      );
    },
    onSuccess: () => {
      mutationKey.current = '';
      setCorrectionOpen(false);
      setCorrectionConfirm(false);
      setCorrectionPoints('');
      setReason('');
      setFeedback(
        `Bonus points added to ${selectedChild?.nickname ?? 'this child'}.`,
      );
      void queryClient.invalidateQueries({ queryKey: ['points', selectedId] });
      void queryClient.invalidateQueries({ queryKey: ['reports', selectedId] });
    },
  });
  const reverse = useMutation({
    mutationFn: () => {
      const item = reverseItem;
      const approvedAttempt = item?.attempts.find(
        (attempt) => attempt.status === 'approved',
      );
      if (!item || !approvedAttempt)
        throw new Error('Approval details are unavailable.');
      if (!mutationKey.current) mutationKey.current = crypto.randomUUID();
      return reviewApi.reverse(
        approvedAttempt.id,
        item.version,
        reason.trim(),
        mutationKey.current,
      );
    },
    onSuccess: () => {
      mutationKey.current = '';
      setReverseItem(null);
      setReason('');
      setFeedback(
        'Approval updated. The original award remains in the activity record.',
      );
      void queryClient.invalidateQueries({ queryKey: ['points', selectedId] });
      void queryClient.invalidateQueries({ queryKey: ['reports', selectedId] });
    },
  });
  const mutationError = correction.error ?? reverse.error;
  const summary = useMemo(
    () =>
      report.data
        ? [
            ['Assigned', report.data.assigned],
            ['Submitted', report.data.submitted],
            ['Waiting', report.data.pending],
            ['Approved', report.data.approved],
            ['Approval updated', report.data.reversed],
            ['Ready to try', report.data.rejected],
            ['Still to do', report.data.incomplete],
            ['Cancelled', report.data.cancelled],
            ['Points earned', report.data.pointsEarned],
            ['Points redeemed', report.data.pointsRedeemed ?? 0],
            ['Points refunded', report.data.pointsRefunded ?? 0],
            ['Bonus points', report.data.manualCorrections],
            ['Net points change', report.data.netPointsChange],
          ]
        : [],
    [report.data],
  );

  if (children.isPending || household.isPending)
    return (
      <div className="route-status" role="status">
        Loading reports…
      </div>
    );
  if (children.isError || household.isError)
    return (
      <div className="notice error" role="alert">
        We could not load reports.{' '}
        <button
          type="button"
          onClick={() => {
            void children.refetch();
            void household.refetch();
          }}
        >
          Try again
        </button>
      </div>
    );
  if (!children.data?.length)
    return (
      <div className="empty-state">
        <div>
          <h1>Reports</h1>
          <p>Add a child before viewing progress.</p>
          <Link className="button button-primary" to="/parent/children">
            Add a child
          </Link>
        </div>
      </div>
    );

  function submitCorrection(event: FormEvent) {
    event.preventDefault();
    mutationKey.current = '';
    setCorrectionOpen(false);
    setCorrectionConfirm(true);
  }

  return (
    <section className="page-stack reports-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Progress and history</p>
          <h1>{selectedChild?.nickname}’s report</h1>
        </div>
        {balance.isError ? (
          <span className="inline-error" role="alert">
            Points unavailable.{' '}
            <button type="button" onClick={() => void balance.refetch()}>
              Try again
            </button>
          </span>
        ) : (
          <strong className="balance-pill">
            {balance.isPending
              ? 'Loading points…'
              : `${balance.data?.points ?? '—'} points`}
          </strong>
        )}
      </div>
      {feedback && (
        <div className="notice success" role="status">
          {feedback}
        </div>
      )}
      {mutationError && (
        <div className="notice error" role="alert">
          {messageForError(mutationError)} You can safely try again.
        </div>
      )}
      <label className="form-field report-child">
        Child
        <select
          value={selectedId}
          onChange={(event) => {
            setChildId(event.target.value);
            setFeedback('');
          }}
        >
          {children.data.map((child) => (
            <option value={child.id} key={child.id}>
              {child.nickname}
            </option>
          ))}
        </select>
      </label>
      <div className="period-tabs" aria-label="Report period">
        {(['day', 'week', 'month'] as const).map((value) => (
          <button
            type="button"
            aria-pressed={period === value}
            className="button button-secondary"
            key={value}
            onClick={() => setPeriod(value)}
          >
            {value.charAt(0).toUpperCase() + value.slice(1)}
          </button>
        ))}
      </div>
      <div className="period-navigation">
        <button
          type="button"
          className="button button-secondary"
          aria-label={`Previous ${period}`}
          onClick={() => setAnchor(shiftAnchor(activeAnchor, period, -1))}
        >
          ←
        </button>
        <strong>
          {report.data ? periodLabel(report.data) : 'Loading period…'}
        </strong>
        <button
          type="button"
          className="button button-secondary"
          aria-label={`Next ${period}`}
          onClick={() => setAnchor(shiftAnchor(activeAnchor, period, 1))}
        >
          →
        </button>
      </div>
      {report.isPending && <div role="status">Loading this period…</div>}
      {report.isError && (
        <div className="notice error" role="alert">
          We could not load this period.{' '}
          <button type="button" onClick={() => void report.refetch()}>
            Try again
          </button>
        </div>
      )}
      {report.data && (
        <dl className="report-grid">
          {summary.map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}

      <section aria-labelledby="history-title">
        <div className="section-heading">
          <div>
            <h2 id="history-title">Recent history</h2>
            <p>At least the last 30 days of assigned work.</p>
          </div>
          <button
            className="button button-secondary"
            type="button"
            onClick={() => {
              setCorrectionOpen(true);
              setReason('');
            }}
          >
            Add bonus points
          </button>
        </div>
        {history.isPending && <div role="status">Loading history…</div>}
        {history.isError && (
          <div className="notice error" role="alert">
            We could not load history.{' '}
            <button type="button" onClick={() => void history.refetch()}>
              Try again
            </button>
          </div>
        )}
        {historyItems.length === 0 && history.isSuccess && (
          <div className="empty-state">
            <p>No activity in this period yet.</p>
          </div>
        )}
        <ol className="activity-list">
          {historyItems.map((item) => (
            <li className="card activity-row" key={item.id}>
              <div>
                <strong>{item.title}</strong>
                <p className="muted">
                  {item.localDate} · {statusLabel(item)} · {item.points} points
                </p>
                {(item.awardDelta !== 0 || item.reversalDelta !== 0) && (
                  <p className="muted">
                    Ledger effect: {item.awardDelta > 0 ? '+' : ''}
                    {item.awardDelta} award · {item.reversalDelta} reversal
                  </p>
                )}
                {item.attempts.length > 0 && (
                  <ul
                    className="attempt-list"
                    aria-label={`Attempts for ${item.title}`}
                  >
                    {item.attempts.map((attempt) => (
                      <li key={attempt.id}>
                        Attempt {attempt.attemptNumber}:{' '}
                        {attempt.status.replace('_', ' ')}
                        {attempt.reason ? ` · Guidance: ${attempt.reason}` : ''}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              {item.status === 'approved' &&
                item.attempts.some(
                  (attempt) => attempt.status === 'approved',
                ) && (
                  <button
                    type="button"
                    className="button button-secondary"
                    onClick={() => {
                      setReverseItem(item);
                      setReason('');
                    }}
                  >
                    Reverse approval
                  </button>
                )}
            </li>
          ))}
        </ol>
        {history.hasNextPage && (
          <button
            className="button button-secondary"
            type="button"
            disabled={history.isFetchingNextPage}
            onClick={() => void history.fetchNextPage()}
          >
            {history.isFetchingNextPage ? 'Loading more…' : 'Load more history'}
          </button>
        )}
        <h3>Point corrections</h3>
        {ledger.isPending && <div role="status">Loading point activity…</div>}
        {ledger.isError && (
          <div className="notice error" role="alert">
            We could not load point activity.{' '}
            <button type="button" onClick={() => void ledger.refetch()}>
              Try again
            </button>
          </div>
        )}
        {ledger.isSuccess &&
          !ledgerItems.some((entry) => entry.kind === 'manual_correction') && (
            <p className="muted">No bonus point corrections yet.</p>
          )}
        <ol className="activity-list">
          {ledgerItems
            .filter((entry) => entry.kind === 'manual_correction')
            .map((entry) => (
              <li className="card activity-row" key={entry.id}>
                <span>
                  <strong>+{entry.amount} bonus points</strong>
                  <br />
                  <span className="muted">{entry.reason}</span>
                </span>
                <time dateTime={entry.createdAt}>
                  {new Intl.DateTimeFormat(undefined, {
                    dateStyle: 'medium',
                  }).format(new Date(entry.createdAt))}
                </time>
              </li>
            ))}
        </ol>
        {ledger.hasNextPage && (
          <button
            className="button button-secondary"
            type="button"
            disabled={ledger.isFetchingNextPage}
            onClick={() => void ledger.fetchNextPage()}
          >
            {ledger.isFetchingNextPage
              ? 'Loading more…'
              : 'Load more point activity'}
          </button>
        )}
      </section>

      {correctionOpen && (
        <AccessibleDialog
          titleId="correction-title"
          onClose={() => setCorrectionOpen(false)}
        >
          <form onSubmit={submitCorrection}>
            <h2 id="correction-title">Add bonus points</h2>
            <p>
              This is an additive, auditable gift. It does not change completed
              work.
            </p>
            <label className="form-field">
              Points
              <input
                data-initial-focus
                required
                type="number"
                min="1"
                max="10000"
                value={correctionPoints}
                onChange={(event) => {
                  mutationKey.current = '';
                  setCorrectionPoints(event.target.value);
                }}
              />
            </label>
            <label className="form-field">
              Reason
              <input
                required
                maxLength={500}
                value={reason}
                onChange={(event) => {
                  mutationKey.current = '';
                  setReason(event.target.value);
                }}
              />
            </label>
            <div className="form-actions">
              <button
                type="button"
                className="button button-secondary"
                onClick={() => setCorrectionOpen(false)}
              >
                Cancel
              </button>
              <button type="submit" className="button button-primary">
                Review bonus
              </button>
            </div>
          </form>
        </AccessibleDialog>
      )}
      {correctionConfirm && (
        <AccessibleDialog
          titleId="correction-confirm-title"
          onClose={() => setCorrectionConfirm(false)}
        >
          <h2 id="correction-confirm-title">Confirm bonus points</h2>
          <p>
            This will change {selectedChild?.nickname ?? 'this child'}’s balance
            by <strong>+{Number(correctionPoints)} points</strong>. The reason
            will be kept in the parent activity record.
          </p>
          <div className="form-actions">
            <button
              data-initial-focus
              type="button"
              className="button button-secondary"
              onClick={() => {
                setCorrectionConfirm(false);
                setCorrectionOpen(true);
              }}
            >
              Go back
            </button>
            <button
              type="button"
              className="button button-primary"
              disabled={correction.isPending}
              aria-busy={correction.isPending}
              onClick={() => correction.mutate()}
            >
              {correction.isPending
                ? 'Adding…'
                : `Confirm +${Number(correctionPoints)} points`}
            </button>
          </div>
        </AccessibleDialog>
      )}
      {reverseItem && (
        <AccessibleDialog
          titleId="reverse-title"
          onClose={() => setReverseItem(null)}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              reverse.mutate();
            }}
          >
            <h2 id="reverse-title">Reverse this approval?</h2>
            <p>
              This is terminal and changes the balance by -
              {reverseItem.awardDelta} points. An offsetting entry will preserve
              the full history; {reverseItem.title} cannot be submitted again.
            </p>
            <label className="form-field">
              Reason
              <input
                data-initial-focus
                required
                maxLength={500}
                value={reason}
                onChange={(event) => {
                  mutationKey.current = '';
                  setReason(event.target.value);
                }}
              />
            </label>
            <div className="form-actions">
              <button
                type="button"
                className="button button-secondary"
                onClick={() => setReverseItem(null)}
              >
                Keep approval
              </button>
              <button
                type="submit"
                className="button button-danger"
                disabled={reverse.isPending}
                aria-busy={reverse.isPending}
              >
                {reverse.isPending ? 'Updating…' : 'Reverse approval'}
              </button>
            </div>
          </form>
        </AccessibleDialog>
      )}
    </section>
  );
}
