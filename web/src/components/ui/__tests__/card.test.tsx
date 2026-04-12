import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'

describe('Card', () => {
  it('renders with children', () => {
    render(<Card><p>Card content</p></Card>)
    expect(screen.getByText('Card content')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(<Card className="my-card">Content</Card>)
    const card = container.firstChild as HTMLElement
    expect(card.className).toContain('my-card')
  })

  it('renders CardHeader', () => {
    render(<CardHeader data-testid="header">Header text</CardHeader>)
    const header = screen.getByTestId('header')
    expect(header).toBeInTheDocument()
    expect(header).toHaveTextContent('Header text')
  })

  it('renders CardTitle as a div element', () => {
    render(<CardTitle data-testid="title">My Title</CardTitle>)
    const title = screen.getByTestId('title')
    expect(title).toBeInTheDocument()
    expect(title.tagName).toBe('DIV')
    expect(title).toHaveTextContent('My Title')
  })

  it('renders CardDescription', () => {
    render(<CardDescription>Some description</CardDescription>)
    expect(screen.getByText('Some description')).toBeInTheDocument()
  })

  it('renders CardContent', () => {
    render(<CardContent data-testid="content">Body here</CardContent>)
    const content = screen.getByTestId('content')
    expect(content).toBeInTheDocument()
    expect(content).toHaveTextContent('Body here')
  })

  it('renders CardFooter', () => {
    render(<CardFooter data-testid="footer">Footer stuff</CardFooter>)
    const footer = screen.getByTestId('footer')
    expect(footer).toBeInTheDocument()
    expect(footer).toHaveTextContent('Footer stuff')
  })

  it('composes full card layout', () => {
    render(
      <Card data-testid="full-card">
        <CardHeader>
          <CardTitle>Title</CardTitle>
          <CardDescription>Description</CardDescription>
        </CardHeader>
        <CardContent>Content</CardContent>
        <CardFooter>Footer</CardFooter>
      </Card>
    )
    expect(screen.getByTestId('full-card')).toBeInTheDocument()
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Description')).toBeInTheDocument()
    expect(screen.getByText('Content')).toBeInTheDocument()
    expect(screen.getByText('Footer')).toBeInTheDocument()
  })
})
