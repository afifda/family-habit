import { describe, expect, it } from 'vitest';

import { ApiError, apiErrorFromResponse, isApiErrorBody } from './errors';

describe('API errors', () => {
  it('parses structured validation errors', async () => {
    const response = new Response(
      JSON.stringify({
        error: {
          code: 'validation_failed',
          message: 'Please correct the highlighted fields.',
          requestId: 'req-1',
          validation: [
            { field: 'nickname', code: 'not_unique', message: 'Already used.' },
          ],
        },
      }),
      { status: 422, headers: { 'Content-Type': 'application/json' } },
    );

    const error = await apiErrorFromResponse(response);

    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({
      status: 422,
      code: 'validation_failed',
      requestId: 'req-1',
      validation: [{ field: 'nickname', code: 'not_unique' }],
    });
  });

  it('uses a safe fallback for non-JSON responses', async () => {
    const response = new Response('gateway unavailable', {
      status: 502,
      statusText: 'Bad Gateway',
      headers: { 'X-Request-ID': 'edge-2' },
    });

    await expect(apiErrorFromResponse(response)).resolves.toMatchObject({
      status: 502,
      code: 'unexpected_response',
      message: 'Bad Gateway',
      requestId: 'edge-2',
    });
  });

  it('rejects malformed validation entries', () => {
    expect(
      isApiErrorBody({
        error: {
          code: 'validation_failed',
          message: 'Invalid',
          validation: [{ field: 'nickname', message: 'Required' }],
        },
      }),
    ).toBe(false);
  });
});
