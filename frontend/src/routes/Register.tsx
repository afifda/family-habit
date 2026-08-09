import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { Link, Navigate, useNavigate } from 'react-router-dom';
import { z } from 'zod';

import { ApiError } from '../api/errors';
import { useAuth } from '../auth/AuthProvider';
import { AuthLayout } from '../components/AuthLayout';
import { FormField, SelectField } from '../components/FormField';

const schema = z.object({
  email: z.string().trim().email('Enter a valid email address.').max(254),
  password: z.string().min(12, 'Use at least 12 characters.').max(128),
  householdName: z.string().trim().min(1, 'Enter a household name.').max(80),
  timezone: z.string().min(1, 'Choose a timezone.'),
  weekStartsOn: z.enum(['sunday', 'monday']),
});
type Values = z.infer<typeof schema>;

const zones = [
  'Asia/Jakarta',
  'Europe/Berlin',
  'Europe/London',
  'America/New_York',
  'America/Los_Angeles',
  'Australia/Sydney',
];

export function Register() {
  const { loading, register: createHousehold, session } = useAuth();
  const navigate = useNavigate();
  const [formError, setFormError] = useState<string>();
  const [step, setStep] = useState<1 | 2>(1);
  const {
    register,
    handleSubmit,
    setError,
    trigger,
    formState: { errors, isSubmitting },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      email: '',
      password: '',
      householdName: '',
      timezone: 'Asia/Jakarta',
      weekStartsOn: 'sunday',
    },
  });

  if (!loading && session?.actor === 'parent' && session.parentMode)
    return <Navigate to="/parent" replace />;

  const submit = handleSubmit(async (values) => {
    setFormError(undefined);
    try {
      await createHousehold(values);
      void navigate('/parent', { replace: true });
    } catch (error) {
      if (error instanceof ApiError) {
        for (const issue of error.validation) {
          if (issue.field in values)
            setError(issue.field as keyof Values, { message: issue.message });
        }
        setFormError(error.message);
      } else
        setFormError('We could not create your household. Please try again.');
    }
  });

  async function continueToHousehold() {
    if (await trigger(['email', 'password'])) setStep(2);
  }

  return (
    <AuthLayout>
      <p className="eyebrow">Household setup</p>
      <h1 id="auth-title">Create your family space</h1>
      <p className="auth-intro">
        Set up the parent account and your household. Nothing is created until
        you finish both steps.
      </p>
      <ol className="step-list" aria-label="Setup progress">
        <li
          className={step === 1 ? 'active' : ''}
          aria-current={step === 1 ? 'step' : undefined}
        >
          1. Parent account
        </li>
        <li
          className={step === 2 ? 'active' : ''}
          aria-current={step === 2 ? 'step' : undefined}
        >
          2. Household
        </li>
      </ol>
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
        {step === 1 ? (
          <>
            <FormField
              id="email"
              label="Parent email"
              type="email"
              autoComplete="email"
              error={errors.email?.message}
              {...register('email')}
            />
            <FormField
              id="password"
              label="Password"
              type="password"
              autoComplete="new-password"
              hint="Use at least 12 characters."
              error={errors.password?.message}
              {...register('password')}
            />
            <button
              className="button button-primary"
              type="button"
              onClick={() => void continueToHousehold()}
            >
              Continue
            </button>
          </>
        ) : (
          <>
            <FormField
              id="householdName"
              label="Household name"
              autoComplete="organization"
              placeholder="The Santoso family"
              error={errors.householdName?.message}
              {...register('householdName')}
            />
            <div className="form-row">
              <SelectField
                id="timezone"
                label="Household timezone"
                hint="Daily tasks follow this local time."
                error={errors.timezone?.message}
                {...register('timezone')}
              >
                {zones.map((zone) => (
                  <option key={zone} value={zone}>
                    {zone.replaceAll('_', ' ')}
                  </option>
                ))}
              </SelectField>
              <SelectField
                id="weekStartsOn"
                label="Week starts on"
                error={errors.weekStartsOn?.message}
                {...register('weekStartsOn')}
              >
                <option value="sunday">Sunday</option>
                <option value="monday">Monday</option>
              </SelectField>
            </div>
            <div className="form-actions">
              <button
                className="button button-secondary"
                type="button"
                onClick={() => setStep(1)}
              >
                Back
              </button>
              <button
                className="button button-primary"
                type="submit"
                disabled={isSubmitting}
              >
                {isSubmitting ? 'Creating household…' : 'Create household'}
              </button>
            </div>
          </>
        )}
      </form>
      <p className="auth-footer">
        Already have an account? <Link to="/login">Sign in</Link>
      </p>
    </AuthLayout>
  );
}
