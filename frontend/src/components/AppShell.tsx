import { NavLink, Outlet, useNavigate } from 'react-router-dom';

import { useAuth } from '../auth/AuthProvider';
import { messages } from '../content/messages';

const navigation = [
  { to: '/parent', label: 'Overview' },
  { to: '/parent/review', label: 'Review' },
  { to: '/parent/children', label: 'Children' },
  { to: '/parent/habits', label: 'Habits & tasks' },
  { to: '/parent/routines', label: 'Routine groups' },
  { to: '/parent/rewards', label: 'Rewards' },
  { to: '/parent/reports', label: 'Reports' },
  { to: '/parent/settings', label: 'Settings' },
] as const;

export function AppShell() {
  const { lockParent, logout } = useAuth();
  const navigate = useNavigate();

  async function signOut() {
    await logout();
    void navigate('/login', { replace: true });
  }

  async function switchProfile() {
    await lockParent();
    void navigate('/', { replace: true });
  }
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      <header className="site-header">
        <NavLink
          className="brand"
          to="/"
          aria-label={`${messages.productName} home`}
        >
          <span className="brand-mark" aria-hidden="true">
            H
          </span>
          <span>
            <strong>{messages.productName}</strong>
            <small>{messages.tagline}</small>
          </span>
        </NavLink>
        <div className="header-actions">
          <button
            className="button button-secondary"
            type="button"
            onClick={() => void switchProfile()}
          >
            {messages.switchProfile}
          </button>
          <button
            className="button button-secondary"
            type="button"
            onClick={() => void signOut()}
          >
            Sign out
          </button>
        </div>
      </header>

      <div className="shell-body">
        <aside className="sidebar" aria-label={messages.navigationLabel}>
          <p className="eyebrow">{messages.parentMode}</p>
          <nav>
            <ul className="nav-list">
              {navigation.map((item) => (
                <li key={item.to}>
                  <NavLink
                    to={item.to}
                    end={item.to === '/parent'}
                    className={({ isActive }) =>
                      isActive ? 'nav-link active' : 'nav-link'
                    }
                  >
                    {item.label}
                  </NavLink>
                </li>
              ))}
            </ul>
          </nav>
        </aside>

        <main className="main-content" id="main-content" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
