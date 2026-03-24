import { Link } from 'react-router';
import { Home, ArrowLeft } from 'lucide-react';

export function NotFound() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-surface p-4">
      <div className="text-center">
        <div className="text-8xl font-bold text-monster-green mb-4">404</div>
        <h1 className="text-2xl font-semibold text-text-primary mb-2">Page Not Found</h1>
        <p className="text-text-secondary mb-8">The page you're looking for doesn't exist or has been moved.</p>
        <div className="flex gap-3 justify-center">
          <button onClick={() => window.history.back()}
            className="flex items-center gap-2 px-4 py-2 border border-border text-text-secondary rounded-lg hover:bg-surface-secondary">
            <ArrowLeft size={16} /> Go Back
          </button>
          <Link to="/"
            className="flex items-center gap-2 px-4 py-2 bg-monster-green text-white rounded-lg hover:bg-monster-green-dark">
            <Home size={16} /> Dashboard
          </Link>
        </div>
      </div>
    </div>
  );
}
