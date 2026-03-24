import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { useAuthStore } from './stores/auth';
import { useThemeStore } from './stores/theme';
import { AppLayout } from './components/layout/AppLayout';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastContainer } from './components/Toast';
import { FullPageLoader } from './components/Spinner';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { NotFound } from './pages/NotFound';
import { Dashboard } from './pages/Dashboard';
import { Apps } from './pages/Apps';
import { AppDetail } from './pages/AppDetail';
import { Marketplace } from './pages/Marketplace';
import { Databases } from './pages/Databases';
import { Servers } from './pages/Servers';
import { Settings } from './pages/Settings';
import { DeployWizard } from './pages/DeployWizard';
import { Domains } from './pages/Domains';
import { Onboarding } from './pages/Onboarding';
import { Team } from './pages/Team';
import { Billing } from './pages/Billing';
import { GitSources } from './pages/GitSources';
import { Backups } from './pages/Backups';
import { Secrets } from './pages/Secrets';
import { Admin } from './pages/Admin';

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
          <Route path="admin" element={<Admin />} />
          <Route path="settings" element={<Settings />} />
        </Route>

        <Route path="*" element={<NotFound />} />
      </Routes>
      <ToastContainer />
    </BrowserRouter>
    </ErrorBoundary>
  );
}
