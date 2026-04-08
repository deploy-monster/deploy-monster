import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { useToast, toast } from '../toastStore';

describe('toastStore', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useToast.setState({ toasts: [] });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts with empty toasts', () => {
    expect(useToast.getState().toasts).toHaveLength(0);
  });

  it('adds a toast with auto-generated id', () => {
    useToast.getState().add({ type: 'success', message: 'Done' });
    const toasts = useToast.getState().toasts;
    expect(toasts).toHaveLength(1);
    expect(toasts[0].type).toBe('success');
    expect(toasts[0].message).toBe('Done');
    expect(toasts[0].id).toBeTruthy();
  });

  it('removes a toast by id', () => {
    useToast.getState().add({ type: 'info', message: 'Hello' });
    const id = useToast.getState().toasts[0].id;
    useToast.getState().remove(id);
    expect(useToast.getState().toasts).toHaveLength(0);
  });

  it('auto-removes toast after default duration', () => {
    useToast.getState().add({ type: 'error', message: 'Oops' });
    expect(useToast.getState().toasts).toHaveLength(1);

    vi.advanceTimersByTime(4000);
    expect(useToast.getState().toasts).toHaveLength(0);
  });

  it('auto-removes toast after custom duration', () => {
    useToast.getState().add({ type: 'info', message: 'Brief', duration: 1000 });
    expect(useToast.getState().toasts).toHaveLength(1);

    vi.advanceTimersByTime(1000);
    expect(useToast.getState().toasts).toHaveLength(0);
  });

  it('supports multiple toasts simultaneously', () => {
    useToast.getState().add({ type: 'success', message: 'First' });
    useToast.getState().add({ type: 'error', message: 'Second' });
    expect(useToast.getState().toasts).toHaveLength(2);
  });

  describe('shorthand helpers', () => {
    it('toast.success adds a success toast', () => {
      toast.success('Saved');
      const t = useToast.getState().toasts;
      expect(t).toHaveLength(1);
      expect(t[0].type).toBe('success');
      expect(t[0].message).toBe('Saved');
    });

    it('toast.error adds an error toast', () => {
      toast.error('Failed');
      const t = useToast.getState().toasts;
      expect(t).toHaveLength(1);
      expect(t[0].type).toBe('error');
    });

    it('toast.info adds an info toast', () => {
      toast.info('FYI');
      const t = useToast.getState().toasts;
      expect(t).toHaveLength(1);
      expect(t[0].type).toBe('info');
    });
  });
});
