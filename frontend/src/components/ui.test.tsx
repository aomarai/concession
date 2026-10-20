import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MemoryRouter } from 'react-router-dom'
import type { WatchlistSummary } from '../api/types'
import Alert from './Alert'
import Avatar from './Avatar'
import Badge from './Badge'
import Button from './Button'
import ListCard from './ListCard'
import Pager from './Pager'
import Poster from './Poster'
import { RatingDisplay, RatingInput } from './Rating'
import SegmentedControl from './SegmentedControl'
import StatusChip from './StatusChip'
import TicketCard, { Fact } from './TicketCard'

describe('Button', () => {
  it('renders variants, sizes and icons as pill buttons that pop', () => {
    render(<>
      <Button>Add a title</Button>
      <Button variant="danger" size="s" icon="plus">Delete list</Button>
      <Button icon="up" aria-label="Move up" />
      <Button type="submit" className="extra">Save</Button>
      <Button variant="glass" size="s" icon="cross" aria-label="Remove" />
    </>)
    expect(screen.getByRole('button', { name: 'Add a title' })).toHaveClass('cn-btn', 'pop')
    expect(screen.getByRole('button', { name: 'Add a title' })).toHaveAttribute('type', 'button')
    expect(screen.getByRole('button', { name: 'Delete list' })).toHaveClass('cn-btn--danger', 'cn-btn--s')
    expect(screen.getByRole('button', { name: 'Move up' })).toHaveClass('cn-btn--icon')
    expect(screen.getByRole('button', { name: 'Save' })).toHaveAttribute('type', 'submit')
    expect(screen.getByRole('button', { name: 'Save' })).toHaveClass('extra')
    expect(screen.getByRole('button', { name: 'Remove' })).toHaveClass('cn-btn--glass', 'cn-btn--icon')
  })

  it('passes clicks through', async () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Go</Button>)
    await userEvent.click(screen.getByRole('button', { name: 'Go' }))
    expect(onClick).toHaveBeenCalledOnce()
  })
})

it('Badge renders its tone and accessible label', () => {
  render(<><Badge label="3 unread">3</Badge><Badge tone="velvet">New</Badge></>)
  expect(screen.getByLabelText('3 unread')).toHaveClass('cn-badge')
  expect(screen.getByText('New')).toHaveClass('cn-badge--velvet')
})

it('StatusChip pairs a status with its word, and ignores unknown values', () => {
  const { container } = render(<><StatusChip status="watching" /><StatusChip status={'nope' as never} /></>)
  expect(screen.getByText('Watching')).toHaveClass('cn-status--watching')
  expect(container.querySelectorAll('.cn-chip')).toHaveLength(1)
})

describe('SegmentedControl', () => {
  function Demo({ start = 'a' }: { start?: string }) {
    const [v, setV] = useState(start)
    return <SegmentedControl label="Show" value={v} onChange={setV} segments={[{ value: 'a', label: 'All' }, { value: 'b', label: 'Watching', name: 'Watching only' }, { value: 'c', label: 'Done' }]} />
  }

  it('is a radio group whose checked option moves on click', async () => {
    render(<Demo />)
    const group = screen.getByRole('radiogroup', { name: 'Show' })
    expect(within(group).getByRole('radio', { name: 'All' })).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(screen.getByRole('radio', { name: 'Watching only' }))
    expect(screen.getByRole('radio', { name: 'Watching only' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'All' })).toHaveAttribute('tabindex', '-1')
  })

  it('moves with arrow keys, wrapping around, and ignores other keys', () => {
    render(<Demo />)
    const all = screen.getByRole('radio', { name: 'All' })
    fireEvent.keyDown(all, { key: 'ArrowLeft' })
    expect(screen.getByRole('radio', { name: 'Done' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Done' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('radio', { name: 'Done' }), { key: 'ArrowRight' })
    expect(all).toHaveAttribute('aria-checked', 'true')
    fireEvent.keyDown(all, { key: 'Tab' })
    expect(all).toHaveAttribute('aria-checked', 'true')
  })

  it('does not report re-choosing the current option', async () => {
    const onChange = vi.fn()
    render(<SegmentedControl label="L" value="a" onChange={onChange} segments={[{ value: 'a', label: 'A' }]} />)
    await userEvent.click(screen.getByRole('radio', { name: 'A' }))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('hides the thumb and keeps the first option focusable when nothing is chosen', () => {
    const { container } = render(<SegmentedControl label="L" value={'z' as string} onChange={() => {}} disabled segments={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />)
    expect((container.querySelector('.cn-seg__thumb') as HTMLElement).style.opacity).toBe('0')
    expect(screen.getByRole('radio', { name: 'A' })).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('radio', { name: 'B' })).toBeDisabled()
  })
})

describe('Rating', () => {
  it('shows a score or "Not rated"', () => {
    render(<><RatingDisplay value={8} /><RatingDisplay size="m" /></>)
    expect(screen.getByRole('img', { name: 'Rated 8 out of 10' })).toHaveTextContent('8')
    expect(screen.getByRole('img', { name: 'Not rated' })).toHaveClass('cn-rating--m')
  })

  function Input({ start = 0 }: { start?: number }) {
    const [v, setV] = useState(start)
    return <RatingInput label="Rating" value={v} onChange={setV} />
  }

  it('is a radio group of ten seats; choosing fills and pops them in turn', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout'] })
    render(<Input />)
    expect(screen.getByText('Choose a seat to rate')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: '1 out of 10' })).toHaveAttribute('tabindex', '0')
    fireEvent.click(screen.getByRole('radio', { name: '7 out of 10' }))
    act(() => { vi.runAllTimers() })
    expect(screen.getByRole('radio', { name: '7 out of 10' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: '7 out of 10' })).toHaveClass('is-on')
    expect(screen.getByRole('radio', { name: '8 out of 10' })).not.toHaveClass('is-on')
    vi.useRealTimers()
  })

  it('previews on hover and steps with arrow keys within 1 to 10', () => {
    render(<Input start={10} />)
    const group = screen.getByRole('radiogroup', { name: 'Rating' })
    fireEvent.mouseEnter(screen.getByRole('radio', { name: '3 out of 10' }))
    expect(screen.getByRole('radio', { name: '9 out of 10' })).not.toHaveClass('is-on')
    fireEvent.mouseLeave(group)
    expect(screen.getByRole('radio', { name: '9 out of 10' })).toHaveClass('is-on')
    fireEvent.keyDown(screen.getByRole('radio', { name: '10 out of 10' }), { key: 'ArrowUp' })
    expect(screen.getByRole('radio', { name: '10 out of 10' })).toHaveAttribute('aria-checked', 'true')
    fireEvent.keyDown(screen.getByRole('radio', { name: '10 out of 10' }), { key: 'ArrowLeft' })
    expect(screen.getByRole('radio', { name: '9 out of 10' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: '9 out of 10' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('radio', { name: '9 out of 10' }), { key: 'Enter' })
    expect(screen.getByRole('radio', { name: '9 out of 10' })).toHaveAttribute('aria-checked', 'true')
  })
})

describe('Poster', () => {
  it('shows the TMDB image at a sharp width for its size', () => {
    render(<><Poster path="/a.jpg" title="Alien" /><Poster path="/b.jpg" title="Big" size="l" /><Poster path="/c.jpg" title="Deco" decorative /></>)
    expect(screen.getByRole('img', { name: 'Alien' })).toHaveAttribute('src', 'https://image.tmdb.org/t/p/w185/a.jpg')
    expect(screen.getByRole('img', { name: 'Big' })).toHaveAttribute('src', 'https://image.tmdb.org/t/p/w500/b.jpg')
    expect(screen.queryByRole('img', { name: 'Deco' })).not.toBeInTheDocument()
  })

  it('draws the initial, skipping a leading article, when there is no poster', () => {
    const { container } = render(<><Poster title="The Thing" size="s" /><Poster title="" /><Poster title="An Ending" decorative /></>)
    expect(screen.getByRole('img', { name: 'The Thing (no poster)' })).toHaveTextContent(/^T$/)
    expect(screen.getByRole('img', { name: '(no poster)' })).toHaveTextContent('?')
    expect(container.querySelector('[aria-hidden="true"].cn-poster--empty')).toHaveTextContent('E')
  })
})

describe('TicketCard', () => {
  it('renders the stub, details, status, rating and actions', () => {
    render(
      <ul>
        <TicketCard title={<a href="/movies/1">The Thing</a>} name="The Thing" meta={<Fact icon="calendar">1982</Fact>} status="completed" rating={9} index={20} tools={<button>Up</button>} actions={<button>Remove</button>}>
          <p>Notes</p>
        </TicketCard>
        <TicketCard title="Bare" name="Bare" />
      </ul>,
    )
    const [first, bare] = screen.getAllByRole('listitem')
    expect(within(first).getByRole('heading', { name: 'The Thing' })).toBeInTheDocument()
    expect(within(first).getByText('1982')).toBeInTheDocument()
    expect(within(first).getByText('Completed')).toBeInTheDocument()
    expect(within(first).getByRole('img', { name: 'Rated 9 out of 10' })).toBeInTheDocument()
    expect(within(first).getByRole('button', { name: 'Remove' })).toBeInTheDocument()
    expect(within(first).getByText('Notes')).toBeInTheDocument()
    expect(within(first).getByRole('button', { name: 'Up' })).toBeInTheDocument()
    expect(bare.querySelector('.cn-ticket__top, .cn-ticket__status, .cn-ticket__actions, .cn-ticket__meta')).toBeNull()
  })
})

describe('ListCard', () => {
  const list: WatchlistSummary = { id: 'l1', owner_id: 'u', title: 'Friday night horror', description: 'Scary ones', privacy: 'shared', type: 'movie', role: 'editor', item_count: 1 }

  it('links its title and tags type, privacy and role', () => {
    render(<MemoryRouter><ul><ListCard list={list} /><ListCard list={{ ...list, id: 'l2', title: 'Shows', description: '', type: 'show', role: 'owner', privacy: 'private', item_count: 3 }} index={2} /></ul></MemoryRouter>)
    expect(screen.getByRole('link', { name: 'Friday night horror' })).toHaveAttribute('href', '/lists/l1')
    const [first, second] = screen.getAllByRole('listitem')
    expect(within(first).getByText('1 title')).toBeInTheDocument()
    expect(within(first).getByText('You can edit')).toBeInTheDocument()
    expect(within(first).getByText('Scary ones')).toBeInTheDocument()
    expect(within(second).getByText('3 titles')).toBeInTheDocument()
    expect(within(second).getByText('TV shows')).toBeInTheDocument()
    expect(within(second).getByText('Private')).toBeInTheDocument()
    expect(within(second).queryByText(/You can edit|View only/)).not.toBeInTheDocument()
  })
})

it('Alert is announced and shows the message', () => {
  render(<Alert message="That list no longer exists." />)
  expect(screen.getByRole('alert')).toHaveTextContent('That list no longer exists.')
})

describe('Pager', () => {
  it('renders nothing for one page and steps between pages', async () => {
    const onChange = vi.fn()
    const { rerender, container } = render(<Pager page={1} total={5} perPage={20} onChange={onChange} />)
    expect(container).toBeEmptyDOMElement()
    rerender(<Pager page={2} total={45} perPage={20} onChange={onChange} />)
    expect(screen.getByText('Page 2 of 3')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Previous' }))
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(onChange.mock.calls).toEqual([[1], [3]])
  })
})

it('Avatar shows a photo or initials', () => {
  const { container } = render(<><Avatar name="Ada Lovelace King" src="/a.png" size="l" /><Avatar name="Grace  Hopper" /><Avatar name="Deco" src="/d.png" decorative /></>)
  expect(screen.getByRole('img', { name: 'Ada Lovelace King' })).toHaveAttribute('src', '/a.png')
  expect(container.querySelector('.cn-avatar--l')).not.toBeNull()
  expect(screen.getByText('GH')).toBeInTheDocument()
  expect(screen.queryByRole('img', { name: 'Deco' })).not.toBeInTheDocument()
})
