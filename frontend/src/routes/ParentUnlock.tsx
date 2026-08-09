import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Navigate, useNavigate } from 'react-router-dom';
import { z } from 'zod';

import { ApiError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';
import { AuthLayout } from '../components/AuthLayout';
import { FormField } from '../components/FormField';

const schema = z.object({
  password: z.string().min(1, 'Enter your password.'),
});
type Values = z.infer<typeof schema>;

export function ParentUnlock() {
  const { loading, session, unlockParent } = useAuth();
  const navigate = useNavigate();
  const [formError, setFormError] = useState<string>();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { password: '' },
  });

  if (!loading && !session) return <Navigate to="/login" replace />;
  if (!loading && session?.actor === 'parent' && session.parentMode)
    return <Navigate to="/parent" replace />;

  const submit = handleSubmit(async (values) => {
    setFormError(undefined);
    try {
      await unlockParent(values);
      void navigate('/parent', { replace: true });
    } catch (error) {
      setFormError(
        error instanceof ApiError && error.status === 429
          ? 'Too many attempts. Wait a moment and try again.'
          : 'That password did not unlock Parent Mode.',
      );
    }
  });

  return (
    <AuthLayout>
      <p className="eyebrow">Shared device protection</p>
      <h1 id="auth-title">Unlock Parent Mode</h1>
      <p className="auth-intro">
        Confirm your parent password to manage the household.
      </p>
      <form
        className="auth-form"
        onSubmit={(event) => void submit(event)}
        noValidate
      >
        {formError && (
          <div className="form-alert" role="alert">
            {formError}
          </div>
        )}
        <FormField
          id="unlock-password"
          label="Parent password"
          type="password"
          autoComplete="current-password"
          error={errors.password?.message}
          {...register('password')}
        />
        <button
          className="button button-primary"
          type="submit"
          disabled={isSubmitting}
        >
          {isSubmitting ? 'Unlocking…' : 'Unlock Parent Mode'}
        </button>
      </form>
    </AuthLayout>
  );
}
