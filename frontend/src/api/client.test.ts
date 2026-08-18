import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  authApi,
  householdApi,
  overviewApi,
  profilesApi,
  rewardsApi,
  reviewApi,
  todayApi,
} from './client';

const session = {
  actor: 'parent' as const,
  householdId: 'd85fa5ac-f090-400b-807b-2a309b384f7f',
  parentMode: true,
  csrfToken: 'csrf-token-at-least-sixteen',
  idleExpiresAt: '2026-08-08T12:00:00Z',
  absoluteExpiresAt: '2026-08-09T12:00:00Z',
};

describe('authentication API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('includes cookie credentials on atomic registration', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        new Response(JSON.stringify({ data: session }), { status: 201 }),
      );
    await authApi.register({
      email: 'p@example.com',
      password: 'long-secure-password',
      householdName: 'Family',
      timezone: 'Asia/Jakarta',
      weekStartsOn: 'sunday',
    });
    const [, init] = fetchMock.mock.calls[0] as [
      RequestInfo | URL,
      RequestInit,
    ];
    expect(init?.credentials).toBe('include');
    expect(new Headers(init?.headers).has('Idempotency-Key')).toBe(false);
  });

  it('uses the bootstrapped CSRF token for logout', async () => {
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: session }), { status: 200 }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    await authApi.session();
    await authApi.logout();
    const [, init] = vi.mocked(fetch).mock.calls[1] as [
      RequestInfo | URL,
      RequestInit,
    ];
    expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe(
      session.csrfToken,
    );
  });
});

describe('profile API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('reads the privacy-limited shared projection endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [], page: { nextCursor: null } }), {
        status: 200,
      }),
    );

    await profilesApi.list();

    const [url] = fetchMock.mock.calls[0] as [string, RequestInit?];
    expect(url).toBe('/api/v1/profiles');
  });
});

describe('Phase 8 parent integration API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('reads the authoritative household-local overview aggregate', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            date: '2026-08-09',
            timezone: 'Asia/Jakarta',
            pending: 0,
            children: [],
          },
        }),
        { status: 200 },
      ),
    );
    await overviewApi.get();
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/parent/overview');
  });

  it('patches complete household preferences through the CSRF-aware client', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'household-1',
            name: 'River Family',
            timezone: 'Europe/Berlin',
            weekStartsOn: 'monday',
            parentModeTimeoutMinutes: 15,
            version: 1,
          },
        }),
        { status: 200 },
      ),
    );
    await householdApi.update({
      name: 'River Family',
      timezone: 'Europe/Berlin',
      weekStartsOn: 'monday',
      parentModeTimeoutMinutes: 15,
      parentPin: '123456',
    });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/household');
    expect(init.method).toBe('PATCH');
    expect(typeof init.body).toBe('string');
    expect(JSON.parse(init.body as string)).toEqual(
      expect.objectContaining({ parentPin: '123456' }),
    );
  });

  it('sends version and a stable caller key for the rewards toggle', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'household-1',
            name: 'River Family',
            timezone: 'Europe/Berlin',
            weekStartsOn: 'monday',
            parentModeTimeoutMinutes: 15,
            rewardsEnabled: true,
            version: 4,
          },
        }),
        { status: 200 },
      ),
    );

    await householdApi.update(
      { rewardsEnabled: true },
      { version: 3, idempotencyKey: 'toggle-rewards-1' },
    );

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(headers.get('If-Match')).toBe('3');
    expect(headers.get('Idempotency-Key')).toBe('toggle-rewards-1');
  });
});

describe('Phase 9 rewards API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('archives rewards through the explicit archive action route', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }));

    await rewardsApi.archive('reward-1', 3);

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/rewards/reward-1/archive');
    expect(init.method).toBe('POST');
    expect(new Headers(init.headers).get('If-Match')).toBe('3');
  });
});

describe('Today API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('sends idempotency and expected occurrence version on submission', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'completion-1',
            occurrenceId: 'occurrence-1',
            childId: 'child-1',
            attemptStatus: 'pending',
            occurrenceStatus: 'pending_approval',
            occurrenceVersion: 4,
            submittedAt: '2026-08-09T08:00:00Z',
          },
        }),
        { status: 201 },
      ),
    );

    await todayApi.submit('occurrence-1', 3, 'same-logical-action');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(url).toBe('/api/v1/occurrences/occurrence-1/completions');
    expect(init.method).toBe('POST');
    expect(headers.get('Idempotency-Key')).toBe('same-logical-action');
    expect(headers.get('If-Match')).toBe('3');
  });

  it('does not add an arbitrary date to the child Today request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            childId: 'child-1',
            date: '2026-08-09',
            timezone: 'Europe/Berlin',
            occurrences: [],
          },
        }),
        { status: 200 },
      ),
    );

    await todayApi.get('child-1');
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/children/child-1/today');
  });

  it('sends the pending occurrence version when withdrawing', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'completion-1',
            occurrenceId: 'occurrence-1',
            childId: 'child-1',
            attemptStatus: 'withdrawn',
            occurrenceStatus: 'not_started',
            occurrenceVersion: 5,
            submittedAt: '2026-08-09T08:00:00Z',
          },
        }),
        { status: 200 },
      ),
    );

    await todayApi.withdraw('completion-1', 4, 'withdraw-action');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(url).toBe('/api/v1/completions/completion-1');
    expect(init.method).toBe('DELETE');
    expect(headers.get('Idempotency-Key')).toBe('withdraw-action');
    expect(headers.get('If-Match')).toBe('4');
  });
});

describe('parent decision API client', () => {
  afterEach(() => vi.restoreAllMocks());

  it('sends the caller-owned retry key and expected version on approval', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            id: 'completion-1',
            occurrenceId: 'occurrence-1',
            childId: 'child-1',
            attemptStatus: 'approved',
            occurrenceStatus: 'approved',
            occurrenceVersion: 3,
            submittedAt: '2026-08-09T08:00:00Z',
          },
        }),
        { status: 200 },
      ),
    );

    await reviewApi.approve('completion-1', 2, 'approval-logical-action');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(url).toBe('/api/v1/completions/completion-1/approve');
    expect(headers.get('Idempotency-Key')).toBe('approval-logical-action');
    expect(headers.get('If-Match')).toBe('2');
  });

  it('forwards opaque review cursors without interpreting them', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [], page: { nextCursor: null } }), {
        status: 200,
      }),
    );
    await reviewApi.pending('child-1', 'opaque+/cursor=');
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/review/pending?childId=child-1&cursor=opaque%2B%2Fcursor%3D',
    );
  });
});
