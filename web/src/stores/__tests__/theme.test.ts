import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useThemeStore } from '../theme';

// jsdom doesn't have matchMedia — provide a mock
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

describe('themeStore', () => {
  beforeEach(() => {
    useThemeStore.setState({ theme: 'system' });
    localStorage.clear();
  });

  it('defaults to system theme', () => {
    expect(useThemeStore.getState().theme).toBe('system');
  });

  it('setTheme updates state and localStorage', () => {
    useThemeStore.getState().setTheme('dark');
    expect(useThemeStore.getState().theme).toBe('dark');
    expect(localStorage.getItem('theme')).toBe('dark');
  });

  it('setTheme to light removes dark class', () => {
    useThemeStore.getState().setTheme('light');
    expect(useThemeStore.getState().theme).toBe('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('setTheme to dark adds dark class', () => {
    useThemeStore.getState().setTheme('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('initialize reads from localStorage', () => {
    localStorage.setItem('theme', 'dark');
    useThemeStore.getState().initialize();
    expect(useThemeStore.getState().theme).toBe('dark');
  });

  it('initialize defaults to system when no saved value', () => {
    useThemeStore.getState().initialize();
    expect(useThemeStore.getState().theme).toBe('system');
  });
});
