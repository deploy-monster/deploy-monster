import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Pagination } from '../Pagination'

describe('Pagination', () => {
  it('renders nothing when totalPages is 1', () => {
    const { container } = render(
      <Pagination page={1} totalPages={1} onPageChange={vi.fn()} />
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders nothing when totalPages is 0', () => {
    const { container } = render(
      <Pagination page={1} totalPages={0} onPageChange={vi.fn()} />
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders page numbers for small page count', () => {
    render(<Pagination page={1} totalPages={3} onPageChange={vi.fn()} />)

    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('highlights the current page with active styling', () => {
    render(<Pagination page={2} totalPages={5} onPageChange={vi.fn()} />)

    const currentButton = screen.getByText('2')
    expect(currentButton.className).toContain('bg-monster-green')
    expect(currentButton.className).toContain('text-white')
  })

  it('does not highlight non-current pages with active styling', () => {
    render(<Pagination page={2} totalPages={5} onPageChange={vi.fn()} />)

    const otherButton = screen.getByText('1')
    expect(otherButton.className).not.toContain('bg-monster-green')
  })

  it('calls onPageChange when a page number is clicked', () => {
    const onPageChange = vi.fn()
    render(<Pagination page={1} totalPages={5} onPageChange={onPageChange} />)

    // With page=1, totalPages=5, visible pages are: 1, 2, ..., 5
    fireEvent.click(screen.getByText('2'))
    expect(onPageChange).toHaveBeenCalledWith(2)
  })

  it('calls onPageChange with next page when next button is clicked', () => {
    const onPageChange = vi.fn()
    render(<Pagination page={2} totalPages={5} onPageChange={onPageChange} />)

    // The next button is the last button in the component
    const buttons = screen.getAllByRole('button')
    const nextButton = buttons[buttons.length - 1]
    fireEvent.click(nextButton)
    expect(onPageChange).toHaveBeenCalledWith(3)
  })

  it('calls onPageChange with previous page when prev button is clicked', () => {
    const onPageChange = vi.fn()
    render(<Pagination page={3} totalPages={5} onPageChange={onPageChange} />)

    const buttons = screen.getAllByRole('button')
    const prevButton = buttons[0]
    fireEvent.click(prevButton)
    expect(onPageChange).toHaveBeenCalledWith(2)
  })

  it('disables previous button on first page', () => {
    render(<Pagination page={1} totalPages={5} onPageChange={vi.fn()} />)

    const buttons = screen.getAllByRole('button')
    const prevButton = buttons[0]
    expect(prevButton).toBeDisabled()
  })

  it('disables next button on last page', () => {
    render(<Pagination page={5} totalPages={5} onPageChange={vi.fn()} />)

    const buttons = screen.getAllByRole('button')
    const nextButton = buttons[buttons.length - 1]
    expect(nextButton).toBeDisabled()
  })

  it('shows ellipsis for large page counts', () => {
    render(<Pagination page={5} totalPages={10} onPageChange={vi.fn()} />)

    // Should show: 1 ... 4 5 6 ... 10
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getAllByText('...')).toHaveLength(2)
  })

  it('shows ellipsis only on right side when on page 2', () => {
    render(<Pagination page={2} totalPages={10} onPageChange={vi.fn()} />)

    // Should show: 1 2 3 ... 10
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getAllByText('...')).toHaveLength(1)
  })

  it('shows ellipsis only on left side when near last page', () => {
    render(<Pagination page={9} totalPages={10} onPageChange={vi.fn()} />)

    // Should show: 1 ... 8 9 10
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('9')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getAllByText('...')).toHaveLength(1)
  })
})
