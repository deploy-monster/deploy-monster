import { useEffect, useState } from 'react';
import { Outlet, useLocation, NavLink } from 'react-router';
import {
  Menu,
  Search,
  Bell,
  ChevronRight,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from '@/components/ui/dropdown-menu';
import { Sidebar } from './Sidebar';
import { SearchDialog } from '../SearchDialog';
import { useAuthStore } from '../../stores/auth';

// ---------------------------------------------------------------------------
// Route metadata for breadcrumbs
// ---------------------------------------------------------------------------

interface RouteMeta {
  title: string;
  parent?: string;
}

const routeMeta: Record<string, RouteMeta> = {
  '/': { title: 'Dashboard' },
  '/apps': { title: 'Applications' },
  '/apps/new': { title: 'Deploy Wizard', parent: '/apps' },
  '/marketplace': { title: 'Marketplace' },
  '/domains': { title: 'Domains' },
  '/databases': { title: 'Databases' },
  '/servers': { title: 'Servers' },
  '/git': { title: 'Git Sources' },
  '/backups': { title: 'Backups' },
  '/secrets': { title: 'Secrets' },
  '/monitoring': { title: 'Monitoring' },
  '/team': { title: 'Team' },
  '/billing': { title: 'Billing' },
  '/admin': { title: 'Admin' },
  '/settings': { title: 'Settings' },
};

/** Build breadcrumb chain from current path */
function buildBreadcrumbs(pathname: string): { label: string; to: string }[] {
  const crumbs: { label: string; to: string }[] = [];

  // Check for dynamic routes (e.g. /apps/:id)
  let currentPath = pathname;
  let meta = routeMeta[currentPath];

  // If no exact match, try prefix patterns
  if (!meta) {
    if (pathname.startsWith('/apps/') && pathname !== '/apps/new') {
      meta = { title: 'App Detail', parent: '/apps' };
    }
  }

  // Walk up the parent chain
  const visited = new Set<string>();
  while (meta && !visited.has(currentPath)) {
    visited.add(currentPath);
    crumbs.unshift({ label: meta.title, to: currentPath });
    if (meta.parent) {
      currentPath = meta.parent;
      meta = routeMeta[currentPath];
    } else {
      break;
    }
  }

  return crumbs;
}

// ---------------------------------------------------------------------------
// AppLayout
// ---------------------------------------------------------------------------

export function AppLayout() {
  const [searchOpen, setSearchOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const location = useLocation();
  const user = useAuthStore((s) => s.user);

  const breadcrumbs = buildBreadcrumbs(location.pathname);
  const pageTitle = breadcrumbs[breadcrumbs.length - 1]?.label || 'DeployMonster';

  // ---- Keyboard shortcuts ----
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      // Cmd/Ctrl+K — search
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === 'Escape') {
        setSearchOpen(false);
      }
    }
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  // Listen for sidebar toggle events (Cmd+B dispatched from Sidebar)
  useEffect(() => {
    function onToggle() {
      setSidebarOpen((prev) => !prev);
    }
    window.addEventListener('toggle-sidebar', onToggle);
    return () => window.removeEventListener('toggle-sidebar', onToggle);
  }, []);

  // Close mobile sidebar on route change
  useEffect(() => {
    const id = requestAnimationFrame(() => setSidebarOpen(false));
    return () => cancelAnimationFrame(id);
  }, [location.pathname]);

  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <div className="flex-1 flex flex-col min-w-0">
        {/* ---- Top bar ---- */}
        <header
          className={cn(
            'sticky top-0 z-40 shrink-0',
            'flex items-center justify-between h-14 px-4 md:px-6',
            'bg-background/80 backdrop-blur-lg',
            'border-b border-border/60',
          )}
        >
          {/* Left side: hamburger + breadcrumbs */}
          <div className="flex items-center gap-3">
            {/* Mobile hamburger */}
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSidebarOpen(true)}
              className="lg:hidden h-9 w-9 text-muted-foreground hover:text-foreground"
            >
              <Menu className="h-5 w-5" />
            </Button>

            {/* Mobile logo */}
            <NavLink to="/" className="flex items-center gap-2 lg:hidden">
              <div
                className={cn(
                  'w-7 h-7 rounded-md bg-primary flex items-center justify-center',
                  'text-primary-foreground font-bold text-[10px]',
                  'shadow-[0_0_8px_rgba(34,197,94,0.2)]',
                )}
              >
                DM
              </div>
            </NavLink>

            {/* Breadcrumbs — desktop */}
            <nav className="hidden lg:flex items-center gap-1.5 text-sm">
              <NavLink
                to="/"
                className="text-muted-foreground hover:text-foreground transition-colors duration-150"
              >
                DeployMonster
              </NavLink>
              {breadcrumbs.map((crumb, idx) => (
                <span key={crumb.to} className="flex items-center gap-1.5">
                  <ChevronRight className="h-3.5 w-3.5 text-muted-foreground/40" />
                  {idx === breadcrumbs.length - 1 ? (
                    <span className="font-medium text-foreground">{crumb.label}</span>
                  ) : (
                    <NavLink
                      to={crumb.to}
                      className="text-muted-foreground hover:text-foreground transition-colors duration-150"
                    >
                      {crumb.label}
                    </NavLink>
                  )}
                </span>
              ))}
            </nav>

            {/* Mobile page title */}
            <span className="lg:hidden text-sm font-medium text-foreground truncate">
              {pageTitle}
            </span>
          </div>

          {/* Right side: search, notifications, avatar */}
          <div className="flex items-center gap-1.5">
            {/* Search — desktop */}
            <Button
              variant="outline"
              size="sm"
              onClick={() => setSearchOpen(true)}
              className={cn(
                'hidden sm:inline-flex gap-2 h-9 px-3',
                'text-muted-foreground border-border/60',
                'hover:bg-accent/50 hover:text-foreground',
                'transition-colors duration-150',
              )}
            >
              <Search className="h-4 w-4" />
              <span className="text-xs">Search...</span>
              <kbd
                className={cn(
                  'pointer-events-none ml-2 hidden sm:inline-flex',
                  'h-5 select-none items-center gap-0.5 rounded border',
                  'bg-muted/50 px-1.5 font-mono text-[10px] font-medium text-muted-foreground',
                )}
              >
                Ctrl+K
              </kbd>
            </Button>

            {/* Search — mobile */}
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSearchOpen(true)}
              className="sm:hidden h-9 w-9 text-muted-foreground hover:text-foreground"
            >
              <Search className="h-[18px] w-[18px]" />
            </Button>

            {/* Notifications bell */}
            <Button
              variant="ghost"
              size="icon"
              className="relative h-9 w-9 text-muted-foreground hover:text-foreground"
            >
              <Bell className="h-[18px] w-[18px]" />
              {/* Notification dot — shown when there are unread notifications */}
              <span className="absolute top-1.5 right-1.5 h-2 w-2 rounded-full bg-primary" />
            </Button>

            {/* User avatar — desktop quick menu */}
            <DropdownMenu>
              <DropdownMenuTrigger className="hidden md:flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-accent/50 transition-colors duration-150">
                <Avatar className="h-7 w-7">
                  <AvatarFallback className="bg-primary/10 text-primary text-xs font-semibold">
                    {user?.name?.[0]?.toUpperCase() || 'U'}
                  </AvatarFallback>
                </Avatar>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-48">
                <DropdownMenuLabel className="font-normal">
                  <div className="flex flex-col space-y-0.5">
                    <p className="text-sm font-medium">{user?.name || 'User'}</p>
                    <p className="text-xs text-muted-foreground truncate">{user?.email}</p>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => window.location.href = '/settings'}>
                  Settings
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => useAuthStore.getState().logout()}
                  className="text-destructive focus:text-destructive"
                >
                  Log out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        {/* ---- Main content ---- */}
        <main className="flex-1 overflow-auto">
          <div className="mx-auto w-full max-w-7xl px-4 py-6 md:px-6 md:py-8 lg:px-8">
            <Outlet />
          </div>
        </main>
      </div>

      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}
