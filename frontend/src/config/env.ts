import { z } from 'zod';

const environmentSchema = z.object({
  VITE_API_BASE_URL: z.string().min(1).default('/api/v1'),
});

const result = environmentSchema.safeParse(import.meta.env);

if (!result.success) {
  throw new Error(`Invalid frontend environment: ${result.error.message}`);
}

export const environment = result.data;
