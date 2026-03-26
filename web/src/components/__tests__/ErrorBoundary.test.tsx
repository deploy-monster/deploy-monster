import { describe, it, expect, vi, beforeAll, afterAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ErrorBoundary } from '../ErrorBoundary'

// Component that throws an error
const ThrowError = ({ shouldThrow }: { shouldThrow: boolean }) => {
  if (shouldThrow) {
    throw new Error('Test error')
  }
  return <div>No error</div>
}

describe('ErrorBoundary', () => {
  // Suppress console.error for cleaner test output
  const originalError = console.error
  beforeAll(() => {
    console.error = vi.fn()
  })
  afterAll(() => {
    console.error = originalError
  })

  it('renders children when no error', () => {
    render(
      <ErrorBoundary>
        <div>Test content</div>
      </ErrorBoundary>
    )
    expect(screen.getByText('Test content')).toBeInTheDocument()
  })

  it('renders fallback UI when error occurs', () => {
    render(
      <ErrorBoundary>
        <ThrowError shouldThrow={true} />
      </ErrorBoundary>
    )
    // ErrorBoundary should render its fallback UI
    expect(screen.queryByText('No error')).not.toBeInTheDocument()
  })
})
