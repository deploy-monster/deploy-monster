import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { Table } from '../Table'

describe('Table', () => {
  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'email', label: 'Email' },
  ]

  const data = [
    { name: 'John Doe', email: 'john@example.com' },
    { name: 'Jane Doe', email: 'jane@example.com' },
  ]

  it('renders table with data', () => {
    render(<Table columns={columns} data={data} />)

    expect(screen.getByText('Name')).toBeInTheDocument()
    expect(screen.getByText('Email')).toBeInTheDocument()
    expect(screen.getByText('John Doe')).toBeInTheDocument()
    expect(screen.getByText('jane@example.com')).toBeInTheDocument()
  })

  it('renders empty message when no data', () => {
    render(<Table columns={columns} data={[]} emptyMessage="No results" />)
    expect(screen.getByText('No results')).toBeInTheDocument()
  })

  it('renders loading state', () => {
    const { container } = render(<Table columns={columns} data={[]} loading />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders with custom empty message', () => {
    render(<Table columns={columns} data={[]} emptyMessage="Custom empty message" />)
    expect(screen.getByText('Custom empty message')).toBeInTheDocument()
  })

  it('renders with keyExtractor', () => {
    const dataWithId = [
      { id: '1', name: 'John', email: 'john@test.com' },
      { id: '2', name: 'Jane', email: 'jane@test.com' },
    ]
    render(
      <Table
        columns={columns}
        data={dataWithId}
        keyExtractor={(item) => item.id as string}
      />
    )
    expect(screen.getByText('John')).toBeInTheDocument()
  })

  it('renders with render function', () => {
    const columnsWithRender = [
      { key: 'name', label: 'Name' },
      {
        key: 'status',
        label: 'Status',
        render: (item: Record<string, unknown>) => (
          <span className="status-badge">{item.status as string}</span>
        ),
      },
    ]
    const dataWithStatus = [
      { name: 'John', status: 'Active' },
    ]
    render(<Table columns={columnsWithRender} data={dataWithStatus} />)
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('calls onRowClick when row is clicked', async () => {
    const handleClick = vi.fn()
    const { container } = render(
      <Table columns={columns} data={data} onRowClick={handleClick} />
    )

    const row = container.querySelector('tbody tr')
    if (row) {
      fireEvent.click(row)
      expect(handleClick).toHaveBeenCalledTimes(1)
    }
  })
})
