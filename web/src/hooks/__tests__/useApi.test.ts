import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useApi, useMutation } from '../useApi'

// Mock the api client module
vi.mock('../../api/client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}))

// Import the mocked module so we can control return values
import { api } from '../../api/client'
const mockedApi = vi.mocked(api)

describe('useApi', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('starts in loading state when immediate is true (default)', () => {
    mockedApi.get.mockReturnValue(new Promise(() => {})) // never resolves
    const { result } = renderHook(() => useApi('/apps'))

    expect(result.current.loading).toBe(true)
    expect(result.current.data).toBeNull()
    expect(result.current.error).toBeNull()
  })

  it('does not fetch when immediate is false', () => {
    const { result } = renderHook(() => useApi('/apps', { immediate: false }))

    expect(result.current.loading).toBe(false)
    expect(result.current.data).toBeNull()
    expect(mockedApi.get).not.toHaveBeenCalled()
  })

  it('returns data on successful fetch', async () => {
    const mockData = [{ id: '1', name: 'my-app' }]
    mockedApi.get.mockResolvedValue(mockData)

    const { result } = renderHook(() => useApi('/apps'))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.data).toEqual(mockData)
    expect(result.current.error).toBeNull()
    expect(mockedApi.get).toHaveBeenCalledWith('/apps')
  })

  it('returns error on failed fetch', async () => {
    mockedApi.get.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useApi('/apps'))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.error).toBe('Network error')
    expect(result.current.data).toBeNull()
  })

  it('returns generic error message for non-Error throws', async () => {
    mockedApi.get.mockRejectedValue('some string error')

    const { result } = renderHook(() => useApi('/apps'))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(result.current.error).toBe('Request failed')
  })

  it('refetch triggers a new request', async () => {
    const mockData = { count: 1 }
    mockedApi.get.mockResolvedValue(mockData)

    const { result } = renderHook(() => useApi('/stats'))

    await waitFor(() => {
      expect(result.current.loading).toBe(false)
    })

    expect(mockedApi.get).toHaveBeenCalledTimes(1)

    await act(async () => {
      await result.current.refetch()
    })

    expect(mockedApi.get).toHaveBeenCalledTimes(2)
  })

  it('handles polling with refreshInterval', async () => {
    vi.useFakeTimers()
    const mockData = { status: 'ok' }
    mockedApi.get.mockResolvedValue(mockData)

    renderHook(() => useApi('/health', { refreshInterval: 5000 }))

    // Wait for initial fetch
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })

    expect(mockedApi.get).toHaveBeenCalledTimes(1)

    // Advance past one interval
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })

    expect(mockedApi.get).toHaveBeenCalledTimes(2)

    vi.useRealTimers()
  })
})

describe('useMutation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('starts idle (not loading)', () => {
    const { result } = renderHook(() => useMutation('post', '/apps'))

    expect(result.current.loading).toBe(false)
    expect(result.current.data).toBeNull()
    expect(result.current.error).toBeNull()
  })

  it('returns data on successful mutation', async () => {
    const responseData = { id: '123', name: 'new-app' }
    mockedApi.post.mockResolvedValue(responseData)

    const { result } = renderHook(() => useMutation('post', '/apps'))

    await act(async () => {
      await result.current.mutate({ name: 'new-app' })
    })

    expect(result.current.data).toEqual(responseData)
    expect(result.current.error).toBeNull()
    expect(result.current.loading).toBe(false)
    expect(mockedApi.post).toHaveBeenCalledWith('/apps', { name: 'new-app' })
  })

  it('sets error on failed mutation', async () => {
    mockedApi.post.mockRejectedValue(new Error('Validation failed'))

    const { result } = renderHook(() => useMutation('post', '/apps'))

    await act(async () => {
      try {
        await result.current.mutate({ name: '' })
      } catch {
        // expected
      }
    })

    expect(result.current.error).toBe('Validation failed')
    expect(result.current.loading).toBe(false)
  })

  it('uses the correct HTTP method', async () => {
    mockedApi.patch.mockResolvedValue({ updated: true })

    const { result } = renderHook(() => useMutation('patch', '/apps/123'))

    await act(async () => {
      await result.current.mutate({ name: 'renamed' })
    })

    expect(mockedApi.patch).toHaveBeenCalledWith('/apps/123', { name: 'renamed' })
  })

  it('throws the original error from mutate', async () => {
    const error = new Error('Server error')
    mockedApi.post.mockRejectedValue(error)

    const { result } = renderHook(() => useMutation('post', '/apps'))

    await expect(
      act(async () => {
        await result.current.mutate({})
      })
    ).rejects.toThrow('Server error')
  })
})
