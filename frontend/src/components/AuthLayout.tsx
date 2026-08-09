import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

import { messages } from '../content/messages';

export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <main className="auth-page" id="main-content">
      <section className="auth-card" aria-labelledby="auth-title">
        <Link className="brand auth-brand" to="/">
          <span className="brand-mark" aria-hidden="true">
            H
          </span>
          <span>
            <strong>{messages.productName}</strong>
            <small>{messages.tagline}</small>
          </span>
        </Link>
        {children}
      </section>
    </main>
  );
}
