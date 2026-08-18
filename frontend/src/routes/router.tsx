import { createBrowserRouter } from 'react-router-dom';

import { AppShell } from '../components/AppShell';
import { RequireParent } from '../auth/RequireParent';
import { ChildShell } from './ChildShell';
import { NotFound } from './NotFound';
import { ProfilePicker } from './ProfilePicker';
import { Login } from './Login';
import { Register } from './Register';
import { ParentUnlock } from './ParentUnlock';
import { Children } from './Children';
import { HabitsTasks } from './HabitsTasks';
import { ChildToday } from './ChildToday';
import { ChildPoints } from './ChildPoints';
import { Reports } from './Reports';
import { ReviewQueue } from './ReviewQueue';
import { ParentOverview } from './ParentOverview';
import { HouseholdSettings } from './HouseholdSettings';
import { RoutineGroups } from './RoutineGroups';
import { ParentRewards } from './ParentRewards';
import { ChildRewards } from './ChildRewards';

export const router = createBrowserRouter([
  { path: '/', element: <ProfilePicker /> },
  { path: '/login', element: <Login /> },
  { path: '/register', element: <Register /> },
  { path: '/parent/unlock', element: <ParentUnlock /> },
  {
    element: <RequireParent />,
    children: [
      {
        path: '/parent',
        element: <AppShell />,
        children: [
          {
            index: true,
            element: <ParentOverview />,
          },
          {
            path: 'review',
            element: <ReviewQueue />,
          },
          {
            path: 'children',
            element: <Children />,
          },
          {
            path: 'habits',
            element: <HabitsTasks />,
          },
          {
            path: 'routines',
            element: <RoutineGroups />,
          },
          {
            path: 'rewards',
            element: <ParentRewards />,
          },
          {
            path: 'reports',
            element: <Reports />,
          },
          {
            path: 'settings',
            element: <HouseholdSettings />,
          },
        ],
      },
    ],
  },
  {
    path: '/child',
    element: <ChildShell />,
    children: [
      {
        path: 'today',
        element: <ChildToday />,
      },
      {
        path: 'points',
        element: <ChildPoints />,
      },
      {
        path: 'rewards',
        element: <ChildRewards />,
      },
    ],
  },
  { path: '*', element: <NotFound /> },
]);
