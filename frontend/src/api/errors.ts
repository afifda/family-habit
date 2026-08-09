export type ValidationIssue = {
  field: string;
  code: string;
  message: string;
};

export type ApiErrorBody = {
  error: {
    code: string;
    message: string;
    requestId?: string;
    validation?: ValidationIssue[];
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly validation: ValidationIssue[];

  constructor(status: number, body: ApiErrorBody) {
    super(body.error.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = body.error.code;
    this.requestId = body.error.requestId;
    this.validation = body.error.validation ?? [];
  }
}

export function messageForError(error: unknown): string {
  return error instanceof Error
    ? error.message
    : 'Something went wrong. Please try again.';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isValidationIssue(value: unknown): value is ValidationIssue {
  return (
    isRecord(value) &&
    typeof value.field === 'string' &&
    typeof value.code === 'string' &&
    typeof value.message === 'string'
  );
}

export function isApiErrorBody(value: unknown): value is ApiErrorBody {
  if (!isRecord(value) || !isRecord(value.error)) return false;

  const { error } = value;
  return (
    typeof error.code === 'string' &&
    typeof error.message === 'string' &&
    (error.requestId === undefined || typeof error.requestId === 'string') &&
    (error.validation === undefined ||
      (Array.isArray(error.validation) &&
        error.validation.every(isValidationIssue)))
  );
}

export async function apiErrorFromResponse(
  response: Response,
): Promise<ApiError> {
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    body = undefined;
  }

  if (isApiErrorBody(body)) return new ApiError(response.status, body);

  return new ApiError(response.status, {
    error: {
      code: 'unexpected_response',
      message:
        response.statusText || 'The server returned an unexpected response.',
      requestId: response.headers.get('X-Request-ID') ?? undefined,
    },
  });
}
