import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'

const REFETCH_LIST = ['item_added', 'item_updated', 'item_removed', 'items_reordered', 'resync']

// Keeps a list page current: the backend pushes small "something changed"
// events (never contents) and we refetch. Events are not replayed, so a
// reconnect (a second "ready") also refetches.
export function useListEvents(listId: string) {
  const qc = useQueryClient()
  const navigate = useNavigate()

  useEffect(() => {
    if (typeof EventSource === 'undefined') return
    const es = new EventSource(`/api/v1/watchlists/${listId}/events`, { withCredentials: true })
    const list = () => qc.invalidateQueries({ queryKey: ['watchlist', listId] })
    const members = () => qc.invalidateQueries({ queryKey: ['members', listId] })

    for (const type of REFETCH_LIST) es.addEventListener(type, () => void list())
    es.addEventListener('list_updated', () => {
      void list()
      void qc.invalidateQueries({ queryKey: ['watchlists'] })
    })
    es.addEventListener('members_changed', () => {
      void members()
      void list()
    })
    es.addEventListener('list_deleted', () => {
      es.close()
      qc.removeQueries({ queryKey: ['watchlist', listId] })
      void qc.invalidateQueries({ queryKey: ['watchlists'] })
      navigate('/')
    })

    let connected = false
    es.addEventListener('ready', () => {
      if (connected) {
        void list()
        void members()
      }
      connected = true
    })

    return () => es.close()
  }, [listId, qc, navigate])
}
