import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { ToastContainer } from '../Toast';
import { useToast, toast } from '@/stores/toastStore';

describe('ToastContainer', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // Reset zustand store between tests
    useToast.setState({ toasts: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders nothing when there are no toasts', () => {
    const { container } = render(<ToastContainer />)
    expect(container.querySelector('.fixed')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('shows a success toast message', () => {
    render(<ToastContainer />)

    act(() => {
      toast.success('Operation completed')
    })

    expect(screen.getByText('Operation completed')).toBeInTheDocument()
  })

  it('shows an error toast message', () => {
    render(<ToastContainer />)

    act(() => {
      toast.error('Something went wrong')
    })

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })

  it('shows an info toast message', () => {
    render(<ToastContainer />)

    act(() => {
      toast.info('FYI: update available')
    })

    expect(screen.getByText('FYI: update available')).toBeInTheDocument()
  })

  it('renders multiple toasts simultaneously', () => {
    render(<ToastContainer />)

    act(() => {
      toast.success('First')
      toast.error('Second')
      toast.info('Third')
    })

    expect(screen.getByText('First')).toBeInTheDocument()
    expect(screen.getByText('Second')).toBeInTheDocument()
    expect(screen.getByText('Third')).toBeInTheDocument()
  })

  it('removes a toast when dismiss button is clicked', () => {
    render(<ToastContainer />)

    act(() => {
      toast.success('Dismissable')
    })

    expect(screen.getByText('Dismissable')).toBeInTheDocument()

    const dismissButton = screen.getByRole('button')
    fireEvent.click(dismissButton)

    expect(screen.queryByText('Dismissable')).not.toBeInTheDocument()
  })

  it('auto-dismisses toast after default duration (4000ms)', () => {
    render(<ToastContainer />)

    act(() => {
      toast.success('Auto dismiss')
    })

    expect(screen.getByText('Auto dismiss')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(4000)
    })

    expect(screen.queryByText('Auto dismiss')).not.toBeInTheDocument()
  })

  it('auto-dismisses toast after custom duration', () => {
    render(<ToastContainer />)

    act(() => {
      useToast.getState().add({ type: 'info', message: 'Quick toast', duration: 1000 })
    })

    expect(screen.getByText('Quick toast')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(1000)
    })

    expect(screen.queryByText('Quick toast')).not.toBeInTheDocument()
  })

  it('uses the store remove method directly', () => {
    render(<ToastContainer />)

    act(() => {
      toast.success('To remove')
    })

    const toasts = useToast.getState().toasts
    expect(toasts).toHaveLength(1)

    act(() => {
      useToast.getState().remove(toasts[0].id)
    })

    expect(useToast.getState().toasts).toHaveLength(0)
    expect(screen.queryByText('To remove')).not.toBeInTheDocument()
  })
})
