import { useState, useEffect, useCallback } from 'react';
import { NavLink, useNavigate } from 'react-router';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from '@/components/ui/dropdown-menu';
import {
  LayoutDashboard,
  Rocket,
  Globe,
  Database,
  Server,
  Moon,
  Sun,
  Monitor,
  Store,
  Users,
  CreditCard,
  GitBranch,
  Archive,
  Lock,
  Shield,
  X,
  Activity,
  ChevronDown,
  Settings,
  LogOut,
  MoreVertical,
  Share2,
} from 'lucide-react';
import { useAuthStore } from '../../stores/auth';
import { useThemeStore } from '../../stores/theme';
import { useApi } from '../../hooks';
import type { PaginatedResponse } from '@/api/client';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface NavItem {
  to: string;
  icon: React.ElementType;
  label: string;
  /** Static count or API key for dynamic badge */
  badge?: number | string;
}

interface NavGroup {
  id: string;
  label: string;
  items: NavItem[];
}

// ---------------------------------------------------------------------------
// Nav data
// ---------------------------------------------------------------------------

const navGroups: NavGroup[] = [
  {
    id: 'platform',
    label: 'Platform',
    items: [
      { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
      { to: '/apps', icon: Rocket, label: 'Applications', badge: 'apps' },
      { to: '/topology', icon: Share2, label: 'Topology' },
      { to: '/marketplace', icon: Store, label: 'Marketplace' },
    ],
  },
  {
    id: 'infrastructure',
    label: 'Infrastructure',
    items: [
      { to: '/domains', icon: Globe, label: 'Domains', badge: 'domains' },
      { to: '/databases', icon: Database, label: 'Databases' },
      { to: '/servers', icon: Server, label: 'Servers' },
      { to: '/git', icon: GitBranch, label: 'Git Sources' },
    ],
  },
  {
    id: 'operations',
    label: 'Operations',
    items: [
      { to: '/backups', icon: Archive, label: 'Backups' },
      { to: '/secrets', icon: Lock, label: 'Secrets' },
      { to: '/monitoring', icon: Activity, label: 'Monitoring' },
    ],
  },
  {
    id: 'management',
    label: 'Management',
    items: [
      { to: '/team', icon: Users, label: 'Team' },
      { to: '/billing', icon: CreditCard, label: 'Billing' },
      { to: '/admin', icon: Shield, label: 'Admin' },
    ],
  },
];

// ---------------------------------------------------------------------------
// Theme segment controls
// ---------------------------------------------------------------------------

type ThemeKey = 'light' | 'dark' | 'system';

const themes: { key: ThemeKey; icon: React.ElementType; label: string }[] = [
  { key: 'light', icon: Sun, label: 'Light' },
  { key: 'dark', icon: Moon, label: 'Dark' },
  { key: 'system', icon: Monitor, label: 'System' },
];

// ---------------------------------------------------------------------------
// Badge count hook — fetches app & domain totals
// ---------------------------------------------------------------------------

function listCount<T>(response: PaginatedResponse<T> | T[] | null | undefined): number | undefined {
  if (!response) return undefined;
  if (Array.isArray(response)) return response.length;
  return response.total ?? response.data?.length;
}

function useBadgeCounts(): Record<string, number | undefined> {
  const { data: appsData } = useApi<PaginatedResponse<unknown> | unknown[]>(
    '/apps?page=1&per_page=1',
    { refreshInterval: 60000 },
  );
  const { data: domainsData } = useApi<PaginatedResponse<unknown> | unknown[]>('/domains', {
    refreshInterval: 60000,
  });

  return {
    apps: listCount(appsData),
    domains: listCount(domainsData),
  };
}

// ---------------------------------------------------------------------------
// Sidebar component
// ---------------------------------------------------------------------------

interface SidebarProps {
  open?: boolean;
  onClose?: () => void;
}

export function Sidebar({ open, onClose }: SidebarProps) {
  const { user, logout } = useAuthStore();
  const { theme, setTheme } = useThemeStore();
  const navigate = useNavigate();
  const counts = useBadgeCounts();

  // Collapsible groups — all start expanded
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const toggleGroup = useCallback((id: string) => {
    setCollapsed((prev) => ({ ...prev, [id]: !prev[id] }));
  }, []);

  // Cmd/Ctrl+B to toggle sidebar (emits custom event consumed by AppLayout)
  useEffect(() => {
    function handleKeydown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'b') {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('toggle-sidebar'));
      }
    }
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  }, []);

  // Close mobile sidebar on nav
  const handleNavClick = () => {
    onClose?.();
  };

  // Resolve badge count for a nav item
  const resolveBadge = (badge?: number | string): number | undefined => {
    if (badge === undefined) return undefined;
    if (typeof badge === 'number') return badge;
    return counts[badge];
  };

  // ----------------------------------
  // Sidebar content
  // ----------------------------------
  const sidebarContent = (
    <div className="flex flex-col h-full bg-sidebar">
      {/* ---- Logo ---- */}
      <div className="flex items-center justify-between px-5 h-16 shrink-0">
        <NavLink to="/" className="flex items-center gap-3 group" onClick={handleNavClick}>
          <div
            className={cn(
              'relative w-9 h-9 rounded-lg bg-sidebar-primary flex items-center justify-center',
              'text-sidebar-primary-foreground font-bold text-sm tracking-tight',
              'shadow-[0_0_12px_rgba(34,197,94,0.25)] group-hover:shadow-[0_0_20px_rgba(34,197,94,0.4)]',
              'transition-shadow duration-300',
            )}
          >
            DM
          </div>
          <span className="text-sidebar-foreground font-semibold text-[17px] tracking-tight">
            DeployMonster
          </span>
        </NavLink>

        {/* Mobile close */}
        {onClose && (
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="lg:hidden text-sidebar-foreground/60 hover:text-sidebar-foreground hover:bg-white/5"
          >
            <X className="h-5 w-5" />
          </Button>
        )}
      </div>

      <div className="px-5">
        <Separator className="bg-sidebar-border/60" />
      </div>

      {/* ---- Navigation ---- */}
      <ScrollArea className="flex-1 py-4">
        <nav className="px-3 space-y-1">
          {navGroups.map((group) => {
            const isCollapsed = collapsed[group.id] ?? false;

            return (
              <div key={group.id} className="mb-1">
                {/* Group header */}
                <button
                  type="button"
                  onClick={() => toggleGroup(group.id)}
                  className={cn(
                    'flex items-center justify-between w-full px-3 py-2',
                    'text-[11px] font-semibold uppercase tracking-widest',
                    'text-sidebar-foreground/40 hover:text-sidebar-foreground/60',
                    'transition-colors duration-150 rounded-md',
                  )}
                >
                  {group.label}
                  <ChevronDown
                    className={cn(
                      'h-3.5 w-3.5 transition-transform duration-200',
                      isCollapsed && '-rotate-90',
                    )}
                  />
                </button>

                {/* Group items */}
                <div
                  className={cn(
                    'space-y-0.5 overflow-hidden transition-all duration-200',
                    isCollapsed ? 'max-h-0 opacity-0' : 'max-h-[500px] opacity-100',
                  )}
                >
                  {group.items.map(({ to, icon: Icon, label, badge }) => {
                    const count = resolveBadge(badge);

                    return (
                      <NavLink
                        key={to}
                        to={to}
                        end={to === '/'}
                        onClick={handleNavClick}
                        className={({ isActive }) =>
                          cn(
                            'group/item flex items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium',
                            'transition-all duration-150 border-l-2',
                            isActive
                              ? 'bg-sidebar-primary/10 border-sidebar-primary text-sidebar-primary'
                              : 'border-transparent text-sidebar-foreground/65 hover:bg-white/[0.05] hover:text-sidebar-foreground',
                          )
                        }
                      >
                        <Icon className="h-[18px] w-[18px] shrink-0" />
                        <span className="flex-1 truncate">{label}</span>
                        {count !== undefined && count > 0 && (
                          <Badge
                            variant="secondary"
                            className={cn(
                              'h-5 min-w-[20px] px-1.5 text-[10px] font-semibold',
                              'bg-sidebar-accent text-sidebar-foreground/80 border-0',
                            )}
                          >
                            {count}
                          </Badge>
                        )}
                      </NavLink>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </nav>
      </ScrollArea>

      {/* ---- Bottom section ---- */}
      <div className="shrink-0 mt-auto">
        <div className="px-5">
          <Separator className="bg-sidebar-border/60" />
        </div>

        <div className="px-3 py-3 space-y-2">
          {/* Theme toggle — 3 segmented buttons */}
          <div className="flex items-center bg-sidebar-accent/50 rounded-lg p-1 mx-1">
            {themes.map(({ key, icon: ThIcon, label: thLabel }) => (
              <Tooltip key={key} content={thLabel} side="top">
                <button
                  type="button"
                  onClick={() => setTheme(key)}
                  className={cn(
                    'flex-1 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5',
                    'text-xs font-medium transition-all duration-200',
                    theme === key
                      ? 'bg-sidebar-primary text-sidebar-primary-foreground shadow-sm'
                      : 'text-sidebar-foreground/50 hover:text-sidebar-foreground/80',
                  )}
                >
                  <ThIcon className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">{thLabel}</span>
                </button>
              </Tooltip>
            ))}
          </div>

          {/* User section */}
          <DropdownMenu>
            <DropdownMenuTrigger
              className={cn(
                'flex items-center gap-3 w-full rounded-lg px-3 py-2.5 mx-0',
                'hover:bg-white/[0.05] transition-colors duration-150',
                'text-left',
              )}
            >
              <div className="relative shrink-0">
                <Avatar className="h-8 w-8">
                  <AvatarFallback className="bg-sidebar-primary/20 text-sidebar-primary text-xs font-semibold">
                    {user?.name?.[0]?.toUpperCase() || 'U'}
                  </AvatarFallback>
                </Avatar>
                {/* Online indicator */}
                <span className="absolute bottom-0 right-0 block h-2.5 w-2.5 rounded-full bg-emerald-500 ring-2 ring-sidebar" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-sidebar-foreground truncate">
                  {user?.name || 'User'}
                </p>
                <p className="text-[11px] text-sidebar-foreground/45 truncate">
                  {user?.email || 'user@example.com'}
                </p>
              </div>
              <MoreVertical className="h-4 w-4 text-sidebar-foreground/40 shrink-0" />
            </DropdownMenuTrigger>

            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel className="font-normal">
                <div className="flex flex-col space-y-1">
                  <p className="text-sm font-medium">{user?.name || 'User'}</p>
                  <p className="text-xs text-muted-foreground">{user?.email}</p>
                </div>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => navigate('/settings')}>
                <Settings className="mr-2 h-4 w-4" />
                Settings
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={logout}
                className="text-destructive focus:text-destructive"
              >
                <LogOut className="mr-2 h-4 w-4" />
                Log out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );

  // ----------------------------------
  // Render
  // ----------------------------------
  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={cn(
          'hidden lg:flex w-[260px] shrink-0 h-screen sticky top-0',
          'bg-sidebar border-r border-sidebar-border',
          'transition-all duration-300',
        )}
      >
        {sidebarContent}
      </aside>

      {/* Mobile overlay */}
      <div
        className={cn(
          'fixed inset-0 z-50 lg:hidden',
          'transition-all duration-300',
          open ? 'visible' : 'invisible pointer-events-none',
        )}
      >
        {/* Backdrop */}
        <div
          className={cn(
            'absolute inset-0 bg-black/60 backdrop-blur-sm',
            'transition-opacity duration-300',
            open ? 'opacity-100' : 'opacity-0',
          )}
          onClick={onClose}
        />

        {/* Slide-in sidebar */}
        <aside
          className={cn(
            'absolute left-0 top-0 flex w-[280px] h-screen',
            'bg-sidebar shadow-2xl shadow-black/40',
            'transition-transform duration-300 ease-out',
            open ? 'translate-x-0' : '-translate-x-full',
          )}
        >
          {sidebarContent}
        </aside>
      </div>
    </>
  );
}
