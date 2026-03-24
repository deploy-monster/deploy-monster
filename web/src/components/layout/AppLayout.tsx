import { useEffect, useState } from 'react';
import { Outlet } from 'react-router';
import { Menu } from 'lucide-react';
import { Sidebar } from './Sidebar';
import { SearchDialog } from '../SearchDialog';

export function AppLayout() {
  const [searchOpen, setSearchOpen] = useState(false);
  const [sidebarOpen, setSidebarOpen] = useState(false);

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

  return (
    <div className="flex min-h-screen bg-surface">
      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <div className="flex-1 flex flex-col min-w-0">
        {/* Mobile header */}
        <header className="lg:hidden flex items-center justify-between px-4 py-3 border-b border-border bg-surface sticky top-0 z-40">
          <button
            onClick={() => setSidebarOpen(true)}
            className="p-2 rounded-lg hover:bg-surface-secondary text-text-primary"
          >
            <Menu size={20} />
          </button>
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-monster-green flex items-center justify-center text-white font-bold text-xs">
              DM
            </div>
            <span className="font-semibold text-text-primary">DeployMonster</span>
          </div>
          <button
            onClick={() => setSearchOpen(true)}
            className="p-2 rounded-lg hover:bg-surface-secondary text-text-muted text-sm"
          >
            <kbd className="px-1.5 py-0.5 rounded border border-border text-xs">Ctrl+K</kbd>
          </button>
        </header>

        <main className="flex-1 p-4 md:p-6 overflow-auto">
          <Outlet />
        </main>
      </div>

      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  );
}
