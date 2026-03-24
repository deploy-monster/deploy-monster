import { NavLink } from 'react-router';
import {
  LayoutDashboard,
  Rocket,
  Globe,
  Database,
  Server,
  Settings,
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
} from 'lucide-react';
import { useAuthStore } from '../../stores/auth';
import { useThemeStore } from '../../stores/theme';

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/apps', icon: Rocket, label: 'Applications' },
  { to: '/marketplace', icon: Store, label: 'Marketplace' },
  { to: '/domains', icon: Globe, label: 'Domains' },
  { to: '/databases', icon: Database, label: 'Databases' },
  { to: '/servers', icon: Server, label: 'Servers' },
  { to: '/git', icon: GitBranch, label: 'Git Sources' },
  { to: '/backups', icon: Archive, label: 'Backups' },
  { to: '/secrets', icon: Lock, label: 'Secrets' },
  { to: '/team', icon: Users, label: 'Team' },
  { to: '/billing', icon: CreditCard, label: 'Billing' },
  { to: '/admin', icon: Shield, label: 'Admin' },
  { to: '/settings', icon: Settings, label: 'Settings' },
];

const themeIcons = { light: Sun, dark: Moon, system: Monitor } as const;

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
    <>
      {/* Logo */}
      <div className="flex items-center justify-between px-4 py-5 border-b border-white/10">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg bg-monster-green flex items-center justify-center text-white font-bold text-sm">
            DM
          </div>
          <span className="text-sidebar-text-active font-semibold text-lg">DeployMonster</span>
        </div>
        {/* Mobile close button */}
        {onClose && (
          <button onClick={onClose} className="lg:hidden p-1 rounded hover:bg-sidebar-hover">
            <X size={20} className="text-sidebar-text" />
          </button>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {navItems.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            onClick={handleNavClick}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
                isActive
                  ? 'bg-sidebar-active text-sidebar-text-active'
                  : 'hover:bg-sidebar-hover'
              }`
            }
          >
            <Icon size={18} />
            {label}
          </NavLink>
        ))}
      </nav>

      {/* Bottom */}
      <div className="px-3 py-4 border-t border-white/10 space-y-2">
        <button
          onClick={nextTheme}
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm w-full hover:bg-sidebar-hover transition-colors"
        >
          <ThemeIcon size={18} />
          {theme.charAt(0).toUpperCase() + theme.slice(1)}
        </button>

        <div className="flex items-center gap-3 px-3 py-2">
          <div className="w-7 h-7 rounded-full bg-monster-purple flex items-center justify-center text-white text-xs font-medium">
            {user?.name?.[0]?.toUpperCase() || 'U'}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs text-sidebar-text-active truncate">{user?.name}</p>
            <p className="text-xs text-sidebar-text truncate">{user?.email}</p>
          </div>
          <button onClick={logout} className="text-sidebar-text hover:text-red-400 transition-colors">
            <LogOut size={16} />
          </button>
        </div>
      </div>
    </>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <aside className="hidden lg:flex flex-col w-60 bg-sidebar text-sidebar-text h-screen sticky top-0 shrink-0">
        {sidebarContent}
      </aside>

      {/* Mobile overlay */}
      {open && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="fixed inset-0 bg-black/50" onClick={onClose} />
          <aside className="fixed left-0 top-0 flex flex-col w-64 bg-sidebar text-sidebar-text h-screen z-50 shadow-xl">
            {sidebarContent}
          </aside>
        </div>
      )}
    </>
  );
}
