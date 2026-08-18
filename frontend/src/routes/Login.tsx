import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { z } from 'zod';

import { ApiError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';
import { AuthLayout } from '../components/AuthLayout';
import { FormField } from '../components/FormField';

const schema = z.object({
  email: z.string().trim().email('Enter a valid email address.'),
  password: z.string().min(1, 'Enter your password.'),
});
type Values = z.infer<typeof schema>;

export function Login() {
  const { loading, login, session } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [formError, setFormError] = useState<string>();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  });

  if (!loading && session?.actor === 'parent' && session.parentMode)
    return <Navigate to="/" replace />;

  const submit = handleSubmit(async (values) => {
    setFormError(undefined);
    try {
      await login(values);
      const destination = (location.state as { from?: string } | null)?.from;
      void navigate(destination?.startsWith('/parent') ? destination : '/', {
        replace: true,
      });
    } catch (error) {
      setFormError(
        error instanceof ApiError && error.status === 429
          ? 'Too many attempts. Wait a moment and try again.'
          : 'Email or password is incorrect.',
      );
    }
  });

  return (
    <AuthLayout>
      <p className="eyebrow">Parent access</p>
      <h1 id="auth-title">Welcome back</h1>
      <p className="auth-intro">
        Sign in to manage your family’s habits and points.
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
          id="email"
          label="Email address"
          type="email"
          autoComplete="email"
          error={errors.email?.message}
          {...register('email')}
        />
        <FormField
          id="password"
          label="Password"
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
          {isSubmitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
      <p className="auth-footer">
        New to Habit Home? <Link to="/register">Create your household</Link>
      </p>
    </AuthLayout>
  );
}
