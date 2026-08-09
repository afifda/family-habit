import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { useAuth } from './AuthProvider';

export function RequireParent() {
  const { loading, session } = useAuth();
  const location = useLocation();

  if (loading) {
    return (
      <div className="route-status" role="status">
        Checking your session…
      </div>
    );
  }
  if (!session) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  if (session.actor !== 'parent' || !session.parentMode) {
    return <Navigate to="/parent/unlock" replace />;
  }
  return <Outlet />;
}
