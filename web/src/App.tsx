import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { useAuthStore } from './stores/auth';
import { useThemeStore } from './stores/theme';
import { AppLayout } from './components/layout/AppLayout';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { Dashboard } from './pages/Dashboard';
import { Apps } from './pages/Apps';
import { AppDetail } from './pages/AppDetail';
import { Marketplace } from './pages/Marketplace';
import { Databases } from './pages/Databases';
import { Servers } from './pages/Servers';
import { Settings } from './pages/Settings';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const isLoading = useAuthStore((s) => s.isLoading);

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-surface">
        <div className="w-8 h-8 border-2 border-monster-green border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

function Placeholder({ title }: { title: string }) {
  return (
    <div className="flex items-center justify-center h-64">
      <p className="text-text-muted text-lg">{title} — Coming soon</p>
    </div>
  );
}

export default function App() {
  const initAuth = useAuthStore((s) => s.initialize);
  const initTheme = useThemeStore((s) => s.initialize);

  useEffect(() => {
    initAuth();
    initTheme();
  }, []);

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />

        <Route
          element={
            <ProtectedRoute>
              <AppLayout />
            </ProtectedRoute>
          }
        >
          <Route index element={<Dashboard />} />
          <Route path="apps" element={<Apps />} />
          <Route path="apps/:id" element={<AppDetail />} />
          <Route path="domains" element={<Placeholder title="Domains" />} />
          <Route path="databases" element={<Databases />} />
          <Route path="servers" element={<Servers />} />
          <Route path="marketplace" element={<Marketplace />} />
          <Route path="settings" element={<Settings />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
