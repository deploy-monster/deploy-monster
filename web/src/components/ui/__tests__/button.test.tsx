import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Button } from '@/components/ui/button'

describe('Button', () => {
  it('renders with default variant', () => {
    render(<Button>Click me</Button>)
    const button = screen.getByRole('button', { name: 'Click me' })
    expect(button).toBeInTheDocument()
    expect(button.className).toContain('bg-primary')
  })

  it('renders all variants', () => {
    const variants = ['default', 'destructive', 'outline', 'secondary', 'ghost', 'link'] as const
    for (const variant of variants) {
      const { unmount } = render(<Button variant={variant}>{variant}</Button>)
      expect(screen.getByRole('button', { name: variant })).toBeInTheDocument()
      unmount()
    }
  })

  it('renders all sizes', () => {
    const sizes = ['default', 'sm', 'lg', 'icon'] as const
    for (const size of sizes) {
      const { unmount } = render(<Button size={size}>{size}</Button>)
      expect(screen.getByRole('button', { name: size })).toBeInTheDocument()
      unmount()
    }
  })

  it('handles onClick', () => {
    const handleClick = vi.fn()
    render(<Button onClick={handleClick}>Press</Button>)
    fireEvent.click(screen.getByRole('button', { name: 'Press' }))
    expect(handleClick).toHaveBeenCalledTimes(1)
  })

  it('shows children content', () => {
    render(<Button><span data-testid="child">Inner</span></Button>)
    expect(screen.getByTestId('child')).toBeInTheDocument()
    expect(screen.getByText('Inner')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Button className="my-custom-class">Styled</Button>)
    const button = screen.getByRole('button', { name: 'Styled' })
    expect(button.className).toContain('my-custom-class')
  })

  it('handles disabled state', () => {
    const handleClick = vi.fn()
    render(<Button disabled onClick={handleClick}>Disabled</Button>)
    const button = screen.getByRole('button', { name: 'Disabled' })
    expect(button).toBeDisabled()
    fireEvent.click(button)
    expect(handleClick).not.toHaveBeenCalled()
  })
})
