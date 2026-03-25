import { useEffect, lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { useAuthStore } from './stores/auth';
import { useThemeStore } from './stores/theme';
import { AppLayout } from './components/layout/AppLayout';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastContainer } from './components/Toast';
import { FullPageLoader } from './components/Spinner';

// Lazy-loaded pages (code splitting)
const Login = lazy(() => import('./pages/Login').then(m => ({ default: m.Login })));
const Register = lazy(() => import('./pages/Register').then(m => ({ default: m.Register })));
const Onboarding = lazy(() => import('./pages/Onboarding').then(m => ({ default: m.Onboarding })));
const NotFound = lazy(() => import('./pages/NotFound').then(m => ({ default: m.NotFound })));
const Dashboard = lazy(() => import('./pages/Dashboard').then(m => ({ default: m.Dashboard })));
const Apps = lazy(() => import('./pages/Apps').then(m => ({ default: m.Apps })));
const AppDetail = lazy(() => import('./pages/AppDetail').then(m => ({ default: m.AppDetail })));
const DeployWizard = lazy(() => import('./pages/DeployWizard').then(m => ({ default: m.DeployWizard })));
const Marketplace = lazy(() => import('./pages/Marketplace').then(m => ({ default: m.Marketplace })));
const Domains = lazy(() => import('./pages/Domains').then(m => ({ default: m.Domains })));
const Databases = lazy(() => import('./pages/Databases').then(m => ({ default: m.Databases })));
const Servers = lazy(() => import('./pages/Servers').then(m => ({ default: m.Servers })));
const GitSources = lazy(() => import('./pages/GitSources').then(m => ({ default: m.GitSources })));
const Backups = lazy(() => import('./pages/Backups').then(m => ({ default: m.Backups })));
const Secrets = lazy(() => import('./pages/Secrets').then(m => ({ default: m.Secrets })));
const Team = lazy(() => import('./pages/Team').then(m => ({ default: m.Team })));
const Billing = lazy(() => import('./pages/Billing').then(m => ({ default: m.Billing })));
const Admin = lazy(() => import('./pages/Admin').then(m => ({ default: m.Admin })));
const Settings = lazy(() => import('./pages/Settings').then(m => ({ default: m.Settings })));
const Monitoring = lazy(() => import('./pages/Monitoring').then(m => ({ default: m.Monitoring })));

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const isLoading = useAuthStore((s) => s.isLoading);

  if (isLoading) {
    return <FullPageLoader />;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

export default function App() {
  const initAuth = useAuthStore((s) => s.initialize);
  const initTheme = useThemeStore((s) => s.initialize);

  useEffect(() => {
    initAuth();
    initTheme();
  }, []);

  return (
    <ErrorBoundary>
    <BrowserRouter>
      <Suspense fallback={<FullPageLoader />}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route path="/onboarding" element={<Onboarding />} />

          <Route
            element={
              <ProtectedRoute>
                <AppLayout />
              </ProtectedRoute>
            }
          >
            <Route index element={<Dashboard />} />
            <Route path="apps" element={<Apps />} />
            <Route path="apps/new" element={<DeployWizard />} />
            <Route path="apps/:id" element={<AppDetail />} />
            <Route path="domains" element={<Domains />} />
            <Route path="databases" element={<Databases />} />
            <Route path="servers" element={<Servers />} />
            <Route path="marketplace" element={<Marketplace />} />
            <Route path="team" element={<Team />} />
            <Route path="billing" element={<Billing />} />
            <Route path="git" element={<GitSources />} />
            <Route path="backups" element={<Backups />} />
            <Route path="secrets" element={<Secrets />} />
            <Route path="monitoring" element={<Monitoring />} />
            <Route path="admin" element={<Admin />} />
            <Route path="settings" element={<Settings />} />
          </Route>

          <Route path="*" element={<NotFound />} />
        </Routes>
      </Suspense>
      <ToastContainer />
    </BrowserRouter>
    </ErrorBoundary>
  );
}
