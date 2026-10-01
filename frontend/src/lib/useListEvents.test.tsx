import { act, screen } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { useListEvents } from './useListEvents'
import { FakeEventSource } from '../test/fakeEventSource'
import { renderWithProviders } from '../test/utils'

function Probe({ id = 'l1' }: { id?: string }) {
  useListEvents(id)
  return <p>Probe</p>
}

function mount(id = 'l1') {
  const r = renderWithProviders(
    <Routes>
      <Route path="/lists/:id" element={<Probe id={id} />} />
      <Route path="/" element={<p>Home</p>} />
    </Routes>,
    { route: '/lists/l1' },
  )
  const spy = vi.spyOn(r.client, 'invalidateQueries')
  return { ...r, spy, es: FakeEventSource.last }
}

beforeEach(() => {
  FakeEventSource.reset()
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => vi.unstubAllGlobals())

describe('useListEvents', () => {
  it('opens one credentialed stream per list and closes it on unmount', () => {
    const { es, unmount } = mount()
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(es.url).toBe('/api/v1/watchlists/l1/events')
    expect(es.withCredentials).toBe(true)
    unmount()
    expect(es.closed).toBe(true)
  })

  it.each(['item_added', 'item_updated', 'item_removed', 'items_reordered', 'resync'])(
    'refetches the list on %s',
    (type) => {
      const { es, spy } = mount()
      act(() => es.emit(type, { item_id: 'i1' }))
      expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlist', 'l1'] })
      expect(spy).not.toHaveBeenCalledWith({ queryKey: ['watchlists'] })
    },
  )

  it('also refreshes the lists overview when the list itself changes', () => {
    const { es, spy } = mount()
    act(() => es.emit('list_updated'))
    expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlist', 'l1'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlists'] })
  })

  it('refreshes members and the list when membership changes', () => {
    const { es, spy } = mount()
    act(() => es.emit('members_changed'))
    expect(spy).toHaveBeenCalledWith({ queryKey: ['members', 'l1'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlist', 'l1'] })
  })

  it('leaves the page when the list is deleted', async () => {
    const { es, spy, client } = mount()
    const removed = vi.spyOn(client, 'removeQueries')
    act(() => es.emit('list_deleted'))
    expect(await screen.findByText('Home')).toBeInTheDocument()
    expect(removed).toHaveBeenCalledWith({ queryKey: ['watchlist', 'l1'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlists'] })
    expect(es.closed).toBe(true)
  })

  it('refetches after a reconnect but not on the first ready', () => {
    const { es, spy } = mount()
    act(() => es.emit('ready'))
    expect(spy).not.toHaveBeenCalled()
    act(() => es.emit('ready'))
    expect(spy).toHaveBeenCalledWith({ queryKey: ['watchlist', 'l1'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['members', 'l1'] })
  })

  it('does nothing where EventSource is unavailable', () => {
    vi.unstubAllGlobals()
    mount()
    expect(screen.getByText('Probe')).toBeInTheDocument()
    expect(FakeEventSource.instances).toHaveLength(0)
  })
})
