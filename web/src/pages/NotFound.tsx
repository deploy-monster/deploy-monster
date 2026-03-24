import { Link } from 'react-router';
import { Home, ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';

export function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <div className="text-center max-w-md">
        <div className="text-9xl font-bold text-primary/20 leading-none select-none">
          404
        </div>
        <h1 className="mt-4 text-2xl font-semibold tracking-tight">Page Not Found</h1>
        <p className="mt-2 text-muted-foreground">
          The page you are looking for does not exist or has been moved.
        </p>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Button variant="outline" onClick={() => window.history.back()}>
            <ArrowLeft /> Go Back
          </Button>
          <Link to="/">
            <Button>
              <Home /> Dashboard
            </Button>
          </Link>
        </div>
      </div>
    </div>
  );
}
