import { create } from 'zustand';

type Theme = 'light' | 'dark' | 'system';

interface ThemeState {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  initialize: () => () => void; // returns cleanup function
}

function applyTheme(theme: Theme) {
  const root = document.documentElement;
  if (theme === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    root.classList.toggle('dark', prefersDark);
  } else {
    root.classList.toggle('dark', theme === 'dark');
  }
}

// Module-level listener reference — replaced on each initialize() call.
// Only one listener is active at a time; calling initialize() twice safely
// removes the first before adding the second.
let mediaQueryListener: ((this: MediaQueryList, ev: MediaQueryListEvent) => void) | null = null;

export const useThemeStore = create<ThemeState>((set) => ({
  theme: 'system',

  setTheme: (theme) => {
    localStorage.setItem('theme', theme);
    applyTheme(theme);
    set({ theme });
  },

  initialize: () => {
    const saved = (localStorage.getItem('theme') as Theme) || 'system';
    applyTheme(saved);
    set({ theme: saved });

    // Remove any previous listener before adding a new one.
    if (mediaQueryListener !== null) {
      window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', mediaQueryListener);
    }

    mediaQueryListener = () => {
      const current = (localStorage.getItem('theme') as Theme) || 'system';
      if (current === 'system') {
        applyTheme('system');
      }
    };
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', mediaQueryListener);

    // Return cleanup function — caller should invoke this on unmount.
    return () => {
      if (mediaQueryListener !== null) {
        window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', mediaQueryListener);
        mediaQueryListener = null;
      }
    };
  },
}));
