import { useEffect } from 'react'

// Steps back to the last page that exists when the current one has emptied out
// (for example after removing the only item on the last page).
export function useClampPage(page: number, total: number | undefined, perPage: number | undefined, setPage: (page: number) => void) {
  useEffect(() => {
    if (total === undefined || perPage === undefined) return
    const last = Math.max(1, Math.ceil(total / perPage))
    if (page > last) setPage(last)
  }, [page, total, perPage, setPage])
}
