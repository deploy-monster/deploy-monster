import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Badge } from '@/components/ui/badge'

describe('Badge', () => {
  it('renders with default variant', () => {
    render(<Badge>Default</Badge>)
    const badge = screen.getByText('Default')
    expect(badge).toBeInTheDocument()
    expect(badge.className).toContain('bg-primary')
  })

  it('renders all variants', () => {
    const variants = ['default', 'secondary', 'destructive', 'outline'] as const
    for (const variant of variants) {
      const { unmount } = render(<Badge variant={variant}>{variant}</Badge>)
      expect(screen.getByText(variant)).toBeInTheDocument()
      unmount()
    }
  })

  it('shows children content', () => {
    render(<Badge><span data-testid="inner">Status</span></Badge>)
    expect(screen.getByTestId('inner')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Badge className="extra-class">Styled</Badge>)
    const badge = screen.getByText('Styled')
    expect(badge.className).toContain('extra-class')
  })

  it('renders as a span element', () => {
    render(<Badge data-testid="badge-el">Tag</Badge>)
    const badge = screen.getByTestId('badge-el')
    expect(badge.tagName).toBe('SPAN')
  })

  it('applies destructive variant styles', () => {
    render(<Badge variant="destructive">Error</Badge>)
    const badge = screen.getByText('Error')
    expect(badge.className).toContain('bg-destructive')
  })

  it('applies outline variant styles', () => {
    render(<Badge variant="outline">Outline</Badge>)
    const badge = screen.getByText('Outline')
    expect(badge.className).toContain('text-foreground')
    expect(badge.className).not.toContain('bg-primary')
  })
})
