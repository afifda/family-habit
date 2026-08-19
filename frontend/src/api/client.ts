import { environment } from '../config/env';
import { apiErrorFromResponse } from './errors';

export type Session = {
  actor: 'profile_picker' | 'parent' | 'child';
  userId?: string | null;
  householdId: string;
  childId?: string | null;
  parentMode: boolean;
  csrfToken: string;
  idleExpiresAt: string | null;
  absoluteExpiresAt: string;
};

type SessionResponse = { data: Session };

export type WeekStart =
  | 'sunday'
  | 'monday'
  | 'tuesday'
  | 'wednesday'
  | 'thursday'
  | 'friday'
  | 'saturday';

export const weekStartOptions: Array<{ value: WeekStart; label: string }> = [
  { value: 'sunday', label: 'Sunday' },
  { value: 'monday', label: 'Monday' },
  { value: 'tuesday', label: 'Tuesday' },
  { value: 'wednesday', label: 'Wednesday' },
  { value: 'thursday', label: 'Thursday' },
  { value: 'friday', label: 'Friday' },
  { value: 'saturday', label: 'Saturday' },
];

export type RegisterInput = {
  email: string;
  password: string;
  householdName: string;
  timezone: string;
  weekStartsOn: WeekStart;
};

export type LoginInput = Pick<RegisterInput, 'email' | 'password'>;

export type Household = {
  id: string;
  name: string;
  timezone: string;
  weekStartsOn: WeekStart;
  parentModeTimeoutMinutes: 5 | 15 | 30;
  rewardsEnabled?: boolean;
  version: number;
};

export type HouseholdUpdate = Partial<
  Pick<
    Household,
    | 'name'
    | 'timezone'
    | 'weekStartsOn'
    | 'parentModeTimeoutMinutes'
    | 'rewardsEnabled'
  >
> & { parentPin?: string | null };

export type ParentOverview = {
  date: string;
  timezone: string;
  pending: number;
  children: Array<{
    childId: string;
    nickname: string;
    avatar: ChildAvatar;
    color: string;
    completed: number;
    total: number;
    pending: number;
    approvedPointsToday: number;
    waitingPointsToday: number;
  }>;
};

let csrfToken: string | undefined;

export async function request<T>(
  path: string,
  init: RequestInit = {},
  options: { csrf?: boolean } = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set('Content-Type', 'application/json');
  if (options.csrf && csrfToken) headers.set('X-CSRF-Token', csrfToken);

  const response = await fetch(`${environment.VITE_API_BASE_URL}${path}`, {
    ...init,
    credentials: 'include',
    headers,
  });

  if (!response.ok) throw await apiErrorFromResponse(response);
  if (response.status === 204) return undefined as T;
  const body = (await response.json()) as T;
  const session = (body as SessionResponse).data;
  if (session?.csrfToken) csrfToken = session.csrfToken;
  return body;
}

export const authApi = {
  async session() {
    return (await request<SessionResponse>('/session')).data;
  },
  async register(input: RegisterInput) {
    return (
      await request<SessionResponse>('/auth/register', {
        method: 'POST',
        body: JSON.stringify(input),
      })
    ).data;
  },
  async login(input: LoginInput) {
    return (
      await request<SessionResponse>('/auth/login', {
        method: 'POST',
        body: JSON.stringify(input),
      })
    ).data;
  },
  async logout() {
    await request<void>('/auth/logout', { method: 'POST' }, { csrf: true });
    csrfToken = undefined;
  },
  async unlockParent(input: { password: string } | { pin: string }) {
    return (
      await request<SessionResponse>(
        '/session/parent/unlock',
        { method: 'POST', body: JSON.stringify(input) },
        { csrf: true },
      )
    ).data;
  },
  async lockParent() {
    return (
      await request<SessionResponse>(
        '/session/parent/lock',
        { method: 'POST' },
        { csrf: true },
      )
    ).data;
  },
  async enterChild(input: { childId: string; pin?: string }) {
    return (
      await request<SessionResponse>(
        '/session/child',
        { method: 'POST', body: JSON.stringify(input) },
        { csrf: true },
      )
    ).data;
  },
  async leaveChild() {
    return (
      await request<SessionResponse>(
        '/session/child',
        { method: 'DELETE' },
        { csrf: true },
      )
    ).data;
  },
};

export const householdApi = {
  async get() {
    return (await request<{ data: Household }>('/household')).data;
  },
  async update(
    input: HouseholdUpdate,
    options?: { version: number; idempotencyKey: string },
  ) {
    return (
      await request<{ data: Household }>(
        '/household',
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: options
            ? {
                'If-Match': String(options.version),
                'Idempotency-Key': options.idempotencyKey,
              }
            : undefined,
        },
        { csrf: true },
      )
    ).data;
  },
};

export const overviewApi = {
  async get() {
    return (await request<{ data: ParentOverview }>('/parent/overview')).data;
  },
};

export const profileSummaryApi = {
  async get() {
    return (await request<{ data: ParentOverview }>('/profiles/summary')).data;
  },
};

export const childAvatars = [
  'fox',
  'bear',
  'rabbit',
  'owl',
  'cat',
  'elephant',
  'panda',
  'koala',
] as const;

export type ChildAvatar = (typeof childAvatars)[number];

export type Child = {
  id: string;
  nickname: string;
  avatar: ChildAvatar;
  color: string;
  pinEnabled: boolean;
  active: boolean;
  createdAt: string;
  updatedAt: string;
};

/** The deliberately limited child shape available outside Parent Mode. */
export type Profile = Pick<Child, 'id' | 'nickname' | 'avatar' | 'color'> & {
  pinRequired: boolean;
};

export type ChildInput = Pick<Child, 'nickname' | 'avatar' | 'color'> & {
  pin?: string | null;
};

type ChildResponse = { data: Child };
type ChildrenResponse = {
  data: Child[];
  page: { nextCursor?: string | null };
};

type ProfilesResponse = {
  data: Profile[];
  page: { nextCursor?: string | null };
};

export const profilesApi = {
  async list() {
    return (await request<ProfilesResponse>('/profiles')).data;
  },
};

export const childrenApi = {
  async list(includeArchived = false) {
    const query = includeArchived ? '?includeArchived=true' : '';
    return (await request<ChildrenResponse>(`/children${query}`)).data;
  },
  async create(input: ChildInput) {
    return (
      await request<ChildResponse>(
        '/children',
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': crypto.randomUUID() },
        },
        { csrf: true },
      )
    ).data;
  },
  async update(id: string, input: Partial<ChildInput>) {
    return (
      await request<ChildResponse>(
        `/children/${id}`,
        { method: 'PATCH', body: JSON.stringify(input) },
        { csrf: true },
      )
    ).data;
  },
  async archive(id: string) {
    await request<void>(
      `/children/${id}`,
      { method: 'DELETE' },
      { csrf: true },
    );
  },
};

export type Weekday =
  | 'monday'
  | 'tuesday'
  | 'wednesday'
  | 'thursday'
  | 'friday'
  | 'saturday'
  | 'sunday';

export type Schedule =
  { kind: 'daily' } | { kind: 'weekdays'; weekdays: Weekday[] };

export type Assignment = {
  id: string;
  habitId: string;
  childId: string;
  points: number;
  schedule: Schedule;
  effectiveStartDate: string;
  active: boolean;
  version?: number;
  routineGroupId?: string | null;
  sortOrder?: number;
};

export type Habit = {
  id: string;
  title: string;
  description?: string;
  icon?: string;
  color?: string;
  active: boolean;
  assignments?: Assignment[];
  createdAt: string;
  updatedAt: string;
  version?: number;
};

export type HabitInput = Pick<Habit, 'title'> &
  Partial<Pick<Habit, 'description' | 'icon' | 'color'>>;

export type AssignmentInput = Pick<
  Assignment,
  'points' | 'schedule' | 'effectiveStartDate'
> & { childIds: string[]; routineGroupId?: string | null; sortOrder?: number };

export type OneOffTask = {
  id: string;
  childId: string;
  title: string;
  description?: string;
  dueDate: string;
  points: number;
  status: 'active' | 'cancelled';
  createdAt: string;
  version?: number;
  routineGroupId?: string | null;
  sortOrder?: number;
};

export type TaskInput = Pick<
  OneOffTask,
  'childId' | 'title' | 'dueDate' | 'points'
> &
  Partial<Pick<OneOffTask, 'description' | 'routineGroupId' | 'sortOrder'>>;

type HabitResponse = { data: Habit };
type HabitsResponse = { data: Habit[]; page: { nextCursor?: string | null } };
type AssignmentResponse = { data: Assignment };
type AssignmentsResponse = { data: Assignment[] };
type TaskResponse = { data: OneOffTask };
type TasksResponse = {
  data: OneOffTask[];
  page: { nextCursor?: string | null };
};

export const habitsApi = {
  async list(active?: boolean) {
    const query = active === undefined ? '' : `?active=${String(active)}`;
    return (await request<HabitsResponse>(`/habits${query}`)).data;
  },
  async create(input: HabitInput) {
    return (
      await request<HabitResponse>(
        '/habits',
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': crypto.randomUUID() },
        },
        { csrf: true },
      )
    ).data;
  },
  async update(
    id: string,
    input: HabitInput & { effectiveDate: string },
    version?: number,
  ) {
    return (
      await request<HabitResponse>(
        `/habits/${id}`,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            ...(version ? { 'If-Match': String(version) } : {}),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async deactivate(id: string, effectiveDate: string, version?: number) {
    await request<void>(
      `/habits/${id}?effectiveDate=${encodeURIComponent(effectiveDate)}`,
      {
        method: 'DELETE',
        headers: {
          'Idempotency-Key': crypto.randomUUID(),
          ...(version ? { 'If-Match': String(version) } : {}),
        },
      },
      { csrf: true },
    );
  },
  async assign(habitId: string, input: AssignmentInput) {
    return (
      await request<AssignmentsResponse>(
        `/habits/${habitId}/assignments`,
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': crypto.randomUUID() },
        },
        { csrf: true },
      )
    ).data;
  },
  async updateAssignment(
    id: string,
    input: Pick<Assignment, 'points' | 'schedule'> & {
      effectiveDate: string;
      routineGroupId?: string | null;
      sortOrder?: number;
    },
    version?: number,
  ) {
    return (
      await request<AssignmentResponse>(
        `/assignments/${id}`,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            ...(version ? { 'If-Match': String(version) } : {}),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async deactivateAssignment(
    id: string,
    effectiveDate: string,
    version?: number,
  ) {
    await request<void>(
      `/assignments/${id}?effectiveDate=${encodeURIComponent(effectiveDate)}`,
      {
        method: 'DELETE',
        headers: {
          'Idempotency-Key': crypto.randomUUID(),
          ...(version ? { 'If-Match': String(version) } : {}),
        },
      },
      { csrf: true },
    );
  },
};

export const tasksApi = {
  async list() {
    return (await request<TasksResponse>('/tasks')).data;
  },
  async create(input: TaskInput) {
    return (
      await request<TaskResponse>(
        '/tasks',
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': crypto.randomUUID() },
        },
        { csrf: true },
      )
    ).data;
  },
  async update(
    id: string,
    input: Partial<Omit<TaskInput, 'childId'>>,
    version?: number,
  ) {
    return (
      await request<TaskResponse>(
        `/tasks/${id}`,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            ...(version ? { 'If-Match': String(version) } : {}),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async cancel(id: string, reason: string, version?: number) {
    await request<void>(
      `/tasks/${id}`,
      {
        method: 'DELETE',
        body: JSON.stringify({ reason }),
        headers: {
          'Idempotency-Key': crypto.randomUUID(),
          ...(version ? { 'If-Match': String(version) } : {}),
        },
      },
      { csrf: true },
    );
  },
};

export type OccurrenceStatus =
  | 'not_started'
  | 'pending_approval'
  | 'approved'
  | 'approval_reversed'
  | 'cancelled';

export type Occurrence = {
  id: string;
  childId: string;
  type: 'habit' | 'task';
  localDate: string;
  title: string;
  description?: string;
  icon?: string;
  color?: string;
  dueDate?: string | null;
  points: number;
  version: number;
  status: OccurrenceStatus;
  group: 'to_do' | 'waiting_for_parent' | 'done';
  dueState: 'scheduled_today' | 'overdue' | 'historical';
  completionId?: string | null;
  availableActions: Array<'submit' | 'withdraw'>;
  routineGroup?: Pick<
    RoutineGroup,
    'id' | 'name' | 'icon' | 'color' | 'sortOrder'
  > | null;
  itemSortOrder?: number;
};

export type Today = {
  childId: string;
  date: string;
  timezone: string;
  occurrences: Occurrence[];
};

export type Completion = {
  id: string;
  occurrenceId: string;
  childId: string;
  attemptStatus:
    'pending' | 'withdrawn' | 'approved' | 'rejected' | 'cancelled';
  submittedAt: string;
  decidedAt?: string | null;
  reason?: string | null;
  occurrenceStatus: OccurrenceStatus;
  occurrenceVersion: number;
};

type TodayResponse = { data: Today };
type CompletionResponse = { data: Completion };

export const todayApi = {
  async get(childId: string, date?: string) {
    const query = date ? `?date=${encodeURIComponent(date)}` : '';
    return (await request<TodayResponse>(`/children/${childId}/today${query}`))
      .data;
  },
  async submit(
    occurrenceId: string,
    expectedVersion: number,
    idempotencyKey: string = crypto.randomUUID(),
  ) {
    return (
      await request<CompletionResponse>(
        `/occurrences/${occurrenceId}/completions`,
        {
          method: 'POST',
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(expectedVersion),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async withdraw(
    completionId: string,
    expectedVersion: number,
    idempotencyKey: string = crypto.randomUUID(),
  ) {
    return (
      await request<CompletionResponse>(
        `/completions/${completionId}`,
        {
          method: 'DELETE',
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(expectedVersion),
          },
        },
        { csrf: true },
      )
    ).data;
  },
};

export type ReviewItem = Completion & {
  title: string;
  points: number;
  childName: string;
  childAvatar: ChildAvatar;
  childColor: string;
  type: 'habit' | 'task';
  localDate: string;
  dueDate?: string | null;
  attemptNumber: number;
  availableActions: Array<'approve' | 'reject'>;
};

export type LedgerEntry = {
  id: string;
  childId: string;
  kind:
    | 'award'
    | 'approval_reversal'
    | 'manual_correction'
    | 'reward_redemption'
    | 'reward_refund';
  amount: number;
  reason: string;
  occurrenceId?: string | null;
  title?: string | null;
  displayLabel?: string;
  createdAt: string;
};

export type PointBalance = { childId: string; points: number; asOf: string };

export type HistoryAttempt = {
  id: string;
  attemptNumber: number;
  status: Completion['attemptStatus'];
  submittedAt: string;
  decidedAt?: string | null;
  reason?: string | null;
};

export type HistoryOccurrence = {
  id: string;
  childId: string;
  type: 'habit' | 'task';
  localDate: string;
  dueDate?: string | null;
  title: string;
  description?: string;
  icon?: string;
  color?: string;
  points: number;
  status: OccurrenceStatus;
  version: number;
  attempts: HistoryAttempt[];
  awardDelta: number;
  reversalDelta: number;
};

export type ReportPeriod = 'day' | 'week' | 'month';
export type ChildReport = {
  childId: string;
  period: ReportPeriod;
  startDate: string;
  endDate: string;
  timezone: string;
  assigned: number;
  submitted: number;
  pending: number;
  approved: number;
  reversed: number;
  rejected: number;
  incomplete: number;
  cancelled: number;
  pointsEarned: number;
  manualCorrections: number;
  pointsRedeemed?: number;
  pointsRefunded?: number;
  netPointsChange: number;
  weekStartsOn: 0 | 1 | 2 | 3 | 4 | 5 | 6;
};

export type ApiPage<T> = {
  data: T[];
  page: { nextCursor?: string | null };
};

function pageQuery(input: { childId?: string; cursor?: string }) {
  const params = new URLSearchParams();
  if (input.childId) params.set('childId', input.childId);
  if (input.cursor) params.set('cursor', input.cursor);
  return params.size ? `?${params.toString()}` : '';
}

export const reviewApi = {
  async pending(childId?: string, cursor?: string) {
    return request<ApiPage<ReviewItem>>(
      `/review/pending${pageQuery({ childId, cursor })}`,
    );
  },
  async approve(
    completionId: string,
    expectedVersion: number,
    idempotencyKey: string,
  ) {
    return (
      await request<CompletionResponse>(
        `/completions/${completionId}/approve`,
        {
          method: 'POST',
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(expectedVersion),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async reject(
    completionId: string,
    expectedVersion: number,
    reason: string,
    idempotencyKey: string,
  ) {
    return (
      await request<CompletionResponse>(
        `/completions/${completionId}/reject`,
        {
          method: 'POST',
          body: JSON.stringify({ reason }),
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(expectedVersion),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async reverse(
    completionId: string,
    expectedVersion: number,
    reason: string,
    idempotencyKey: string,
  ) {
    return (
      await request<CompletionResponse>(
        `/completions/${completionId}/reverse`,
        {
          method: 'POST',
          body: JSON.stringify({ reason }),
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(expectedVersion),
          },
        },
        { csrf: true },
      )
    ).data;
  },
};

export const pointsApi = {
  async balance(childId: string) {
    return (
      await request<{ data: PointBalance }>(`/children/${childId}/points`)
    ).data;
  },
  async ledger(childId: string, cursor?: string) {
    const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
    return request<ApiPage<LedgerEntry>>(
      `/children/${childId}/points/ledger${query}`,
    );
  },
  async history(childId: string, from?: string, to?: string, cursor?: string) {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (cursor) params.set('cursor', cursor);
    const query = params.size ? `?${params.toString()}` : '';
    return request<ApiPage<HistoryOccurrence>>(
      `/children/${childId}/occurrences${query}`,
    );
  },
  async correct(
    childId: string,
    points: number,
    reason: string,
    idempotencyKey: string,
  ) {
    return (
      await request<{ data: LedgerEntry }>(
        `/children/${childId}/points/corrections`,
        {
          method: 'POST',
          body: JSON.stringify({ points, reason }),
          headers: { 'Idempotency-Key': idempotencyKey },
        },
        { csrf: true },
      )
    ).data;
  },
};

export const reportsApi = {
  async child(childId: string, period: ReportPeriod, anchorDate: string) {
    const query = new URLSearchParams({ period, anchorDate });
    return (
      await request<{ data: ChildReport }>(
        `/reports/children/${childId}?${query.toString()}`,
      )
    ).data;
  },
};

export type RoutineGroup = {
  id: string;
  name: string;
  icon?: string;
  color: string;
  startsAtLocal?: string | null;
  endsAtLocal?: string | null;
  sortOrder: number;
  archivedAt?: string | null;
  version: number;
};

export type RoutineGroupInput = Pick<RoutineGroup, 'name' | 'color'> &
  Partial<
    Pick<RoutineGroup, 'icon' | 'startsAtLocal' | 'endsAtLocal' | 'sortOrder'>
  >;

export const routineGroupsApi = {
  async list() {
    return (await request<ApiPage<RoutineGroup>>('/routine-groups')).data;
  },
  async create(input: RoutineGroupInput, idempotencyKey = crypto.randomUUID()) {
    return (
      await request<{ data: RoutineGroup }>(
        '/routine-groups',
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': idempotencyKey },
        },
        { csrf: true },
      )
    ).data;
  },
  async update(id: string, input: RoutineGroupInput, version: number) {
    return (
      await request<{ data: RoutineGroup }>(
        `/routine-groups/${id}`,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            'If-Match': String(version),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async archive(
    id: string,
    version: number,
    input: { effectiveFrom: string; moveToRoutineGroupId: string | null },
  ) {
    await request<void>(
      `/routine-groups/${id}/archive`,
      {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'Idempotency-Key': crypto.randomUUID(),
          'If-Match': String(version),
        },
      },
      { csrf: true },
    );
  },
  async reorder(groups: RoutineGroup[]) {
    return request<void>(
      '/routine-groups/order',
      {
        method: 'PUT',
        body: JSON.stringify({
          orderedIds: groups.map(({ id }) => id),
          items: groups.map(({ id, version }) => ({ id, version })),
        }),
        headers: { 'Idempotency-Key': crypto.randomUUID() },
      },
      { csrf: true },
    );
  },
};

export type Reward = {
  id: string;
  title: string;
  description?: string;
  icon?: string;
  costPoints: number;
  eligibleChildIds: string[];
  availabilityScope: 'all_active_children' | 'selected_children';
  active: boolean;
  canRedeem?: boolean;
  shortfallPoints?: number;
  version: number;
};

export type RewardInput = Pick<Reward, 'title' | 'costPoints'> &
  Partial<Pick<Reward, 'description' | 'icon' | 'eligibleChildIds'>> & {
    availabilityScope: Reward['availabilityScope'];
  };

export type ChildReward = Pick<
  Reward,
  'id' | 'title' | 'description' | 'icon' | 'costPoints' | 'version'
> & {
  canRedeem: boolean;
  shortfallPoints?: number;
};

export type RewardRedemption = {
  id: string;
  childId: string;
  rewardId: string;
  rewardTitle: string;
  costPoints: number;
  state: 'requested' | 'fulfilled' | 'cancelled';
  requestedAt: string;
  decidedAt?: string | null;
  cancellationReason?: string | null;
  version: number;
};

export type RewardEligibilityPolicy = {
  enabled: boolean;
  period: 'daily' | 'weekly' | 'monthly';
  minimumPoints: number;
  minimumCompletionPercentage: number | null;
  maximumRedemptions: number | null;
  graceHours: 0 | 12 | 24 | 48;
  effectiveFrom: string | null;
  version: number;
};

export type RewardEligibilityRuleResult = {
  type: 'minimum_points' | 'minimum_completion_percentage';
  target: number;
  actual: number;
  passed: boolean;
};

export type RewardEligibilityStatus =
  'collecting' | 'awaiting_evaluation' | 'eligible' | 'not_eligible';

export type RewardEligibilityProgress = {
  childId: string;
  childName: string;
  policyEnabled: boolean;
  collectionPeriodStart: string;
  collectionPeriodEnd: string;
  evaluationAt: string | null;
  pointsCollected: number;
  completionPercentage: number | null;
  assignedCount: number;
  approvedCount: number;
  status: RewardEligibilityStatus;
  eligibleFrom: string | null;
  eligibleUntil: string | null;
  redemptionsUsed: number;
  maximumRedemptions: number | null;
  rules: RewardEligibilityRuleResult[];
};

export type RewardEligibilityEvaluation = RewardEligibilityProgress & {
  id: string;
  evaluatedAt: string;
};

export type ChildRewardEligibility = {
  policyEnabled: boolean;
  status: RewardEligibilityStatus;
  collectionPeriodStart: string | null;
  collectionPeriodEnd: string | null;
  pointsCollected: number;
  minimumPoints: number;
  completionPercentage: number | null;
  minimumCompletionPercentage: number | null;
  eligibleFrom: string | null;
  eligibleUntil: string | null;
  redemptionsUsed: number;
  maximumRedemptions: number | null;
  canRedeem: boolean;
  unavailableReason: string | null;
  pointsShortfall: number;
};

export const rewardsApi = {
  async list() {
    return (await request<ApiPage<Reward>>('/rewards')).data;
  },
  async create(input: RewardInput, idempotencyKey = crypto.randomUUID()) {
    return (
      await request<{ data: Reward }>(
        '/rewards',
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: { 'Idempotency-Key': idempotencyKey },
        },
        { csrf: true },
      )
    ).data;
  },
  async update(id: string, input: RewardInput, version: number) {
    return (
      await request<{ data: Reward }>(
        `/rewards/${id}`,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            'If-Match': String(version),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async archive(id: string, version: number) {
    await request<void>(
      `/rewards/${id}/archive`,
      {
        method: 'POST',
        headers: {
          'Idempotency-Key': crypto.randomUUID(),
          'If-Match': String(version),
        },
      },
      { csrf: true },
    );
  },
  async childCatalog() {
    return request<{
      data: ChildReward[];
      balance: number;
      eligibility?: ChildRewardEligibility;
    }>('/child/rewards');
  },
  async redeem(
    id: string,
    rewardVersion: number,
    confirmedCostPoints: number,
    idempotencyKey: string,
  ) {
    return (
      await request<{ data: RewardRedemption }>(
        `/child/rewards/${id}/redemptions`,
        {
          method: 'POST',
          body: JSON.stringify({
            rewardVersion,
            confirmedCostPoints,
          }),
          headers: { 'Idempotency-Key': idempotencyKey },
        },
        { csrf: true },
      )
    ).data;
  },
  async childRedemptions() {
    return (
      await request<ApiPage<RewardRedemption>>('/child/reward-redemptions')
    ).data;
  },
  async redemptions(status?: RewardRedemption['state']) {
    const query = status ? `?state=${status}` : '';
    return (
      await request<ApiPage<RewardRedemption>>(`/reward-redemptions${query}`)
    ).data;
  },
  async fulfill(item: RewardRedemption) {
    return (
      await request<{ data: RewardRedemption }>(
        `/reward-redemptions/${item.id}/fulfill`,
        {
          method: 'POST',
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            'If-Match': String(item.version),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async cancel(item: RewardRedemption, reason: string) {
    return (
      await request<{ data: RewardRedemption }>(
        `/reward-redemptions/${item.id}/cancel`,
        {
          method: 'POST',
          body: JSON.stringify({ reason }),
          headers: {
            'Idempotency-Key': crypto.randomUUID(),
            'If-Match': String(item.version),
          },
        },
        { csrf: true },
      )
    ).data;
  },
};

export const rewardEligibilityApi = {
  async policy() {
    return (
      await request<{ data: RewardEligibilityPolicy }>(
        '/reward-eligibility-policy',
      )
    ).data;
  },
  async updatePolicy(
    input: Omit<RewardEligibilityPolicy, 'version' | 'effectiveFrom'>,
    version: number,
    idempotencyKey: string,
  ) {
    return (
      await request<{ data: RewardEligibilityPolicy }>(
        '/reward-eligibility-policy',
        {
          method: 'PUT',
          body: JSON.stringify(input),
          headers: {
            'Idempotency-Key': idempotencyKey,
            'If-Match': String(version),
          },
        },
        { csrf: true },
      )
    ).data;
  },
  async progress() {
    return (
      await request<{ data: RewardEligibilityProgress[] }>(
        '/reward-eligibility-progress',
      )
    ).data;
  },
  async evaluations(childId?: string) {
    const query = childId ? `?childId=${encodeURIComponent(childId)}` : '';
    return (
      await request<ApiPage<RewardEligibilityEvaluation>>(
        `/reward-eligibility-evaluations${query}`,
      )
    ).data;
  },
};
