import { renderHook } from '@testing-library/react'
import { useClampPage } from './useClampPage'

describe('useClampPage', () => {
  it('moves back to the last page when the current one no longer exists', () => {
    const setPage = vi.fn()
    renderHook(() => useClampPage(3, 41, 20, setPage))
    expect(setPage).not.toHaveBeenCalled()
    const shrunk = vi.fn()
    renderHook(() => useClampPage(3, 40, 20, shrunk))
    expect(shrunk).toHaveBeenCalledWith(2)
  })

  it('goes to page 1 when nothing is left', () => {
    const setPage = vi.fn()
    renderHook(() => useClampPage(2, 0, 20, setPage))
    expect(setPage).toHaveBeenCalledWith(1)
  })

  it('does nothing while the total is unknown', () => {
    const setPage = vi.fn()
    renderHook(() => useClampPage(2, undefined, 20, setPage))
    expect(setPage).not.toHaveBeenCalled()
  })
})
