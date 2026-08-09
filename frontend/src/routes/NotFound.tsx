import { Link } from 'react-router-dom';

import { messages } from '../content/messages';

export function NotFound() {
  return (
    <main className="profile-page" id="main-content">
      <section className="profile-card" aria-labelledby="not-found-heading">
        <p className="eyebrow">Error 404</p>
        <h1 id="not-found-heading">{messages.notFoundTitle}</h1>
        <p className="page-intro">{messages.notFoundBody}</p>
        <Link className="button button-primary" to="/">
          Return to profile picker
        </Link>
      </section>
    </main>
  );
}
