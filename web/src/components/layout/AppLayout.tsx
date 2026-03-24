import { useEffect, useState } from 'react';
import { Outlet, useLocation } from 'react-router';
import { Menu, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Sidebar } from './Sidebar';
import { SearchDialog } from '../SearchDialog';

const routeTitles: Record<string, string> = {
  '/': 'Dashboard',
  '/apps': 'Applications',
  '/apps/new': 'Deploy Wizard',
  '/marketplace': 'Marketplace',
  '/domains': 'Domains',
  '/databases': 'Databases',
  '/servers': 'Servers',
  '/git': 'Git Sources',
  '/backups': 'Backups',
  '/secrets': 'Secrets',
  '/monitoring': 'Monitoring',
  '/team': 'Team',
  '/billing': 'Billing',
  '/admin': 'Admin',
  '/settings': 'Settings',
};

export function AppLayout() {
  const [searchOpen, setSearchOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const location = useLocation();

  const pageTitle = routeTitles[location.pathname] || 'DeployMonster';

  // CMD+K / CTRL+K keyboard shortcut for search
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setSearchOpen(true);
      }
      if (e.key === 'Escape') {
        setSearchOpen(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  // Close sidebar on route change (mobile)
  useEffect(() => {
    setSidebarOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <div className="flex-1 flex flex-col min-w-0">
        {/* Top bar */}
        <header className="sticky top-0 z-40 flex items-center justify-between h-14 px-4 md:px-6 bg-background/95 backdrop-blur-sm border-b border-border">
          <div className="flex items-center gap-3">
            {/* Mobile hamburger */}
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSidebarOpen(true)}
              className="lg:hidden"
            >
              <Menu className="h-5 w-5" />
            </Button>

            {/* Mobile logo */}
            <div className="flex items-center gap-2 lg:hidden">
              <div className="w-7 h-7 rounded-md bg-primary flex items-center justify-center text-primary-foreground font-bold text-xs">
                DM
              </div>
            </div>

            {/* Breadcrumb / page title */}
            <div className="hidden lg:flex items-center gap-2 text-sm">
              <span className="text-muted-foreground">DeployMonster</span>
              <span className="text-muted-foreground/50">/</span>
              <span className="font-medium text-foreground">{pageTitle}</span>
            </div>
          </div>

          {/* Right side actions */}
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setSearchOpen(true)}
              className="hidden sm:inline-flex gap-2 text-muted-foreground"
            >
              <Search className="h-4 w-4" />
              <span className="text-xs">Search...</span>
              <kbd className="pointer-events-none ml-2 hidden h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground sm:flex">
                Ctrl+K
              </kbd>
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSearchOpen(true)}
              className="sm:hidden"
            >
              <Search className="h-5 w-5" />
            </Button>
          </div>
        </header>

        {/* Main content */}
        <main className="flex-1 p-4 md:p-6 lg:p-8 overflow-auto">
          <Outlet />
        </main>
      </div>

      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}
