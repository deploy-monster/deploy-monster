import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Input } from '@/components/ui/input'

describe('Input', () => {
  it('renders with placeholder', () => {
    render(<Input placeholder="Enter text..." />)
    const input = screen.getByPlaceholderText('Enter text...')
    expect(input).toBeInTheDocument()
  })

  it('handles value change', () => {
    const handleChange = vi.fn()
    render(<Input placeholder="Type here" onChange={handleChange} />)
    const input = screen.getByPlaceholderText('Type here')
    fireEvent.change(input, { target: { value: 'hello' } })
    expect(handleChange).toHaveBeenCalledTimes(1)
  })

  it('handles disabled state', () => {
    render(<Input placeholder="Disabled" disabled />)
    const input = screen.getByPlaceholderText('Disabled')
    expect(input).toBeDisabled()
  })

  it('applies custom className', () => {
    render(<Input placeholder="Styled" className="my-input" />)
    const input = screen.getByPlaceholderText('Styled')
    expect(input.className).toContain('my-input')
  })

  it('handles type prop', () => {
    render(<Input type="password" placeholder="Password" />)
    const input = screen.getByPlaceholderText('Password')
    expect(input).toHaveAttribute('type', 'password')
  })

  it('renders as an input element', () => {
    render(<Input placeholder="Check tag" />)
    const input = screen.getByPlaceholderText('Check tag')
    expect(input.tagName).toBe('INPUT')
  })

  it('accepts an initial value', () => {
    render(<Input placeholder="Default type" defaultValue="prefilled" />)
    const input = screen.getByPlaceholderText('Default type') as HTMLInputElement
    expect(input.value).toBe('prefilled')
  })
})
