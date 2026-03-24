import { NavLink } from 'react-router';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import {
  LayoutDashboard,
  Rocket,
  Globe,
  Database,
  Server,
  LogOut,
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
} from 'lucide-react';
import { useAuthStore } from '../../stores/auth';
import { useThemeStore } from '../../stores/theme';

interface NavGroup {
  label: string;
  items: {
    to: string;
    icon: React.ElementType;
    label: string;
  }[];
}

const navGroups: NavGroup[] = [
  {
    label: 'Platform',
    items: [
      { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
      { to: '/apps', icon: Rocket, label: 'Applications' },
      { to: '/marketplace', icon: Store, label: 'Marketplace' },
    ],
  },
  {
    label: 'Infrastructure',
    items: [
      { to: '/domains', icon: Globe, label: 'Domains' },
      { to: '/databases', icon: Database, label: 'Databases' },
      { to: '/servers', icon: Server, label: 'Servers' },
      { to: '/git', icon: GitBranch, label: 'Git Sources' },
    ],
  },
  {
    label: 'Operations',
    items: [
      { to: '/backups', icon: Archive, label: 'Backups' },
      { to: '/secrets', icon: Lock, label: 'Secrets' },
      { to: '/monitoring', icon: Activity, label: 'Monitoring' },
    ],
  },
  {
    label: 'Management',
    items: [
      { to: '/team', icon: Users, label: 'Team' },
      { to: '/billing', icon: CreditCard, label: 'Billing' },
      { to: '/admin', icon: Shield, label: 'Admin' },
    ],
  },
];

const themeIcons = { light: Sun, dark: Moon, system: Monitor } as const;
const themeLabels = { light: 'Light', dark: 'Dark', system: 'System' } as const;

interface SidebarProps {
  open?: boolean;
  onClose?: () => void;
}

export function Sidebar({ open, onClose }: SidebarProps) {
  const { user, logout } = useAuthStore();
  const { theme, setTheme } = useThemeStore();

  const nextTheme = () => {
    const order: Array<'light' | 'dark' | 'system'> = ['light', 'dark', 'system'];
    const idx = order.indexOf(theme);
    setTheme(order[(idx + 1) % order.length]);
  };

  const ThemeIcon = themeIcons[theme];

  const handleNavClick = () => {
    if (onClose) onClose();
  };

  const sidebarContent = (
    <div className="flex flex-col h-full">
      {/* Logo section */}
      <div className="flex items-center justify-between px-5 h-16 shrink-0">
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-sidebar-primary flex items-center justify-center text-sidebar-primary-foreground font-bold text-sm">
            DM
          </div>
          <span className="text-sidebar-foreground font-semibold text-lg tracking-tight">
            DeployMonster
          </span>
        </div>
        {onClose && (
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="lg:hidden text-sidebar-foreground hover:bg-sidebar-accent"
          >
            <X className="h-5 w-5" />
          </Button>
        )}
      </div>

      <Separator className="bg-sidebar-border" />

      {/* Navigation groups */}
      <nav className="flex-1 overflow-y-auto px-3 py-4 space-y-6">
        {navGroups.map((group) => (
          <div key={group.label}>
            <p className="px-3 mb-2 text-[11px] font-semibold uppercase tracking-wider text-sidebar-foreground/50">
              {group.label}
            </p>
            <div className="space-y-0.5">
              {group.items.map(({ to, icon: Icon, label }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === '/'}
                  onClick={handleNavClick}
                  className={({ isActive }) =>
                    cn(
                      'group flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                      isActive
                        ? 'bg-sidebar-accent text-sidebar-accent-foreground border-l-2 border-sidebar-primary'
                        : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground border-l-2 border-transparent'
                    )
                  }
                >
                  <Icon className="h-4 w-4 shrink-0" />
                  {label}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      {/* Bottom section */}
      <div className="shrink-0 px-3 pb-4 space-y-2">
        <Separator className="bg-sidebar-border mb-3" />

        {/* Theme toggle */}
        <button
          onClick={nextTheme}
          className="flex items-center gap-3 w-full rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground transition-colors"
        >
          <ThemeIcon className="h-4 w-4 shrink-0" />
          {themeLabels[theme]}
        </button>

        {/* User section */}
        <div className="flex items-center gap-3 rounded-md px-3 py-2">
          <Avatar className="h-8 w-8">
            <AvatarFallback className="bg-sidebar-primary text-sidebar-primary-foreground text-xs font-semibold">
              {user?.name?.[0]?.toUpperCase() || 'U'}
            </AvatarFallback>
          </Avatar>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-sidebar-foreground truncate">
              {user?.name || 'User'}
            </p>
            <p className="text-xs text-sidebar-foreground/50 truncate">
              {user?.email}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={logout}
            className="h-8 w-8 text-sidebar-foreground/50 hover:text-destructive hover:bg-sidebar-accent"
          >
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <aside className="hidden lg:flex w-64 bg-sidebar text-sidebar-foreground h-screen sticky top-0 shrink-0 border-r border-sidebar-border">
        {sidebarContent}
      </aside>

      {/* Mobile overlay */}
      {open && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div
            className="fixed inset-0 bg-black/60 backdrop-blur-sm"
            onClick={onClose}
          />
          <aside className="fixed left-0 top-0 flex w-72 bg-sidebar text-sidebar-foreground h-screen z-50 shadow-2xl animate-in slide-in-from-left duration-200">
            {sidebarContent}
          </aside>
        </div>
      )}
    </>
  );
}
