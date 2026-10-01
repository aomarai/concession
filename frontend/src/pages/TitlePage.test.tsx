import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import TitlePage from './TitlePage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { Movie, Review, ReviewPage, Show, TitleKind } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const me = { id: 'u1', username: 'ada', display_name: 'Ada L', avatar_url: '' }
const movie: Movie = {
  id: 1, tmdb_id: 603, title: 'The Matrix', overview: 'A hacker learns the truth.', tagline: 'Free your mind',
  poster_path: '/m.jpg', release_date: '1999-03-30T00:00:00Z', runtime: 136,
  actors: ['Keanu Reeves', 'Carrie-Anne Moss'], genres: [{ id: 1, name: 'Action' }, { id: 2, name: 'Sci-Fi' }],
}
const show: Show = {
  id: 2, tmdb_id: 1396, name: 'Breaking Bad', overview: 'A teacher cooks.',
  seasons: [{ id: 1, season_number: 1, title: 'Season 1' }, { id: 2, season_number: 2, title: 'Season 2' }],
}
const review = (o: Partial<Review> = {}): Review => ({
  id: 'r1', rating: 9, title: 'Great', content: 'Loved it', author: { id: 'u2', display_name: 'Grace H' },
  created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', ...o,
})
const page = (reviews: Review[], o: Partial<ReviewPage> = {}): ReviewPage => ({
  reviews, summary: { count: reviews.length, average: 9 }, page: 1, per_page: 20, total: reviews.length, ...o,
})

function renderTitle(kind: TitleKind = 'movies') {
  return renderWithProviders(
    <Routes><Route path={`/${kind}/:tmdbId`} element={<TitlePage kind={kind} />} /></Routes>,
    { route: kind === 'movies' ? '/movies/603' : '/shows/1396' },
  )
}

beforeEach(() => {
  vi.mocked(api.getMe).mockResolvedValue(me)
  vi.mocked(api.getMovie).mockResolvedValue(movie)
  vi.mocked(api.getShow).mockResolvedValue(show)
  vi.mocked(api.listReviews).mockResolvedValue(page([]))
  vi.mocked(api.getProgress).mockResolvedValue(null)
})

describe('TitlePage details', () => {
  it('shows a movie', async () => {
    renderTitle()
    expect(await screen.findByRole('heading', { name: /The Matrix/ })).toBeInTheDocument()
    expect(api.getMovie).toHaveBeenCalledWith(603)
    expect(screen.getByText('1999 · 136 min')).toBeInTheDocument()
    expect(screen.getByText('Free your mind')).toBeInTheDocument()
    expect(screen.getByText('A hacker learns the truth.')).toBeInTheDocument()
    expect(screen.getByText('Action, Sci-Fi')).toBeInTheDocument()
    expect(screen.getByText(/Keanu Reeves, Carrie-Anne Moss/)).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'The Matrix' })).toBeInTheDocument()
  })

  it('shows a movie with minimal metadata', async () => {
    vi.mocked(api.getMovie).mockResolvedValue({ id: 1, tmdb_id: 603, title: 'Bare', overview: '', poster_path: '', release_date: '' })
    renderTitle()
    expect(await screen.findByRole('heading', { name: 'Bare' })).toBeInTheDocument()
    expect(screen.queryByText(/min/)).not.toBeInTheDocument()
  })

  it('shows a show with its seasons', async () => {
    renderTitle('shows')
    expect(await screen.findByRole('heading', { name: 'Breaking Bad' })).toBeInTheDocument()
    expect(api.getShow).toHaveBeenCalledWith(1396)
    expect(screen.getByText('2 seasons')).toBeInTheDocument()
    expect(screen.getByText('A teacher cooks.')).toBeInTheDocument()
  })

  it('shows a show without extra metadata', async () => {
    vi.mocked(api.getShow).mockResolvedValue({ id: 2, name: 'Bare Show', overview: '' })
    renderTitle('shows')
    expect(await screen.findByRole('heading', { name: 'Bare Show' })).toBeInTheDocument()
    expect(screen.queryByText(/seasons/)).not.toBeInTheDocument()
  })

  it('singularizes one season', async () => {
    vi.mocked(api.getShow).mockResolvedValue({ ...show, seasons: [show.seasons![0]] })
    renderTitle('shows')
    expect(await screen.findByText('1 season')).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.getMovie).mockRejectedValue(new ApiError(404, 'not_found', 'No such title'))
    renderTitle()
    expect(await screen.findByRole('alert')).toHaveTextContent('No such title')
  })
})

describe('TitlePage reviews', () => {
  it('lists reviews with the average', async () => {
    vi.mocked(api.listReviews).mockResolvedValue(page([review(), review({ id: 'r2', title: '', content: '', rating: 7 })]))
    renderTitle()
    expect((await screen.findAllByText('Grace H')).length).toBe(2)
    expect(await screen.findByText('Great')).toBeInTheDocument()
    expect(screen.getByText('Loved it')).toBeInTheDocument()
    expect(screen.getByText('9/10')).toBeInTheDocument()
    expect(screen.getByText('7/10')).toBeInTheDocument()
    expect(screen.getByText('9 average · 2 reviews')).toBeInTheDocument()
    expect(api.listReviews).toHaveBeenCalledWith('movies', 603, 1)
  })

  it('shows an empty state without a summary', async () => {
    vi.mocked(api.listReviews).mockResolvedValue({ reviews: [], page: 1, per_page: 20, total: 0 })
    renderTitle()
    expect(await screen.findByText(/no reviews yet/i)).toBeInTheDocument()
  })

  it('singularizes one review', async () => {
    vi.mocked(api.listReviews).mockResolvedValue(page([review()]))
    renderTitle()
    expect(await screen.findByText('9 average · 1 review')).toBeInTheDocument()
  })

  it('shows review load errors', async () => {
    vi.mocked(api.listReviews).mockRejectedValue(new ApiError(500, 'internal', 'reviews broke'))
    renderTitle()
    expect(await screen.findByText('reviews broke')).toBeInTheDocument()
  })

  it('pages through reviews', async () => {
    vi.mocked(api.listReviews).mockResolvedValue(page([review()], { total: 45 }))
    renderTitle()
    await screen.findByText('Great')
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(api.listReviews).toHaveBeenLastCalledWith('movies', 603, 2)
    await screen.findByText('Great')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('Great')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(api.listReviews).toHaveBeenLastCalledWith('movies', 603, 2)
  })

  it('posts a review and refreshes', async () => {
    vi.mocked(api.createReview).mockResolvedValue(review())
    renderTitle('shows')
    await screen.findByText(/no reviews yet/i)
    vi.mocked(api.listReviews).mockResolvedValue(page([review({ id: 'r9', author: { id: 'u1', display_name: 'Ada L' }, title: 'Mine' })]))

    await userEvent.selectOptions(screen.getByLabelText('Rating'), '9')
    await userEvent.type(screen.getByLabelText('Headline'), 'Mine')
    await userEvent.type(screen.getByLabelText('Review'), 'So good')
    await userEvent.click(screen.getByRole('button', { name: 'Post review' }))

    expect(api.createReview).toHaveBeenCalledWith('shows', 1396, { rating: 9, title: 'Mine', content: 'So good' })
    expect(await screen.findByText('Your review')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Post review' })).not.toBeInTheDocument()
  })

  it('requires a rating before posting', async () => {
    renderTitle()
    await screen.findByText(/no reviews yet/i)
    await userEvent.click(screen.getByRole('button', { name: 'Post review' }))
    expect(api.createReview).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent(/choose a rating/i)
  })

  it('shows post errors', async () => {
    vi.mocked(api.createReview).mockRejectedValue(new ApiError(409, 'conflict', 'Already reviewed'))
    renderTitle()
    await screen.findByText(/no reviews yet/i)
    await userEvent.selectOptions(screen.getByLabelText('Rating'), '5')
    await userEvent.click(screen.getByRole('button', { name: 'Post review' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Already reviewed')
  })
})

describe('TitlePage own review', () => {
  const mine = () => review({ id: 'r9', rating: 8, title: 'Mine', content: 'Solid', author: { id: 'u1', display_name: 'Ada L' } })
  beforeEach(() => {
    vi.mocked(api.listReviews).mockResolvedValue(page([mine(), review()]))
  })

  it('marks it and offers edit and delete instead of the form', async () => {
    renderTitle()
    expect(await screen.findByText('Your review')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Post review' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Edit' })).toHaveLength(1)
  })

  it('edits it', async () => {
    vi.mocked(api.updateReview).mockResolvedValue(mine())
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    expect(screen.getByLabelText('Rating')).toHaveValue('8')
    expect(screen.getByLabelText('Headline')).toHaveValue('Mine')
    await userEvent.selectOptions(screen.getByLabelText('Rating'), '10')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(api.updateReview).toHaveBeenCalledWith('r9', { rating: 10, title: 'Mine', content: 'Solid' })
    expect(await screen.findByRole('button', { name: 'Edit' })).toBeInTheDocument()
  })

  it('cancels editing', async () => {
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument()
    expect(api.updateReview).not.toHaveBeenCalled()
  })

  it('deletes it after confirming', async () => {
    vi.mocked(api.deleteReview).mockResolvedValue()
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(api.deleteReview).not.toHaveBeenCalled()
    vi.mocked(api.listReviews).mockResolvedValue(page([review()]))
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    expect(api.deleteReview).toHaveBeenCalledWith('r9')
    expect(await screen.findByRole('button', { name: 'Post review' })).toBeInTheDocument()
  })

  it('can back out of deleting', async () => {
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await userEvent.click(screen.getByRole('button', { name: 'Keep' }))
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument()
  })

  it('shows delete errors', async () => {
    vi.mocked(api.deleteReview).mockRejectedValue(new ApiError(404, 'not_found', 'Gone already'))
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    await userEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    const card = screen.getByText('Your review').closest('li')!
    expect(await within(card).findByRole('alert')).toHaveTextContent('Gone already')
  })

  it('shows edit errors', async () => {
    vi.mocked(api.updateReview).mockRejectedValue(new ApiError(400, 'bad_request', 'Rating out of range'))
    renderTitle()
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Rating out of range')
  })
})

describe('TitlePage watch progress', () => {
  it('starts untracked and tracks a movie', async () => {
    vi.mocked(api.setProgress).mockResolvedValue({ status: 'plan_to_watch', last_season_num: 0, last_episode_num: 0 })
    renderTitle()
    const select = await screen.findByLabelText('Your status')
    expect(select).toHaveValue('')
    expect(screen.queryByLabelText('Season')).not.toBeInTheDocument()
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'plan_to_watch', last_season_num: 0, last_episode_num: 0 })
    await userEvent.selectOptions(select, 'plan_to_watch')
    expect(api.setProgress).toHaveBeenCalledWith('movies', 603, { status: 'plan_to_watch' })
    await waitFor(() => expect(screen.getByLabelText('Your status')).toHaveValue('plan_to_watch'))
  })

  it('changes and clears the status', async () => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 0, last_episode_num: 0 })
    vi.mocked(api.setProgress).mockResolvedValue({ status: 'completed', last_season_num: 0, last_episode_num: 0 })
    vi.mocked(api.clearProgress).mockResolvedValue()
    renderTitle()
    const select = await screen.findByLabelText('Your status')
    await waitFor(() => expect(select).toHaveValue('watching'))
    await userEvent.selectOptions(select, 'completed')
    expect(api.setProgress).toHaveBeenCalledWith('movies', 603, { status: 'completed' })
    await userEvent.selectOptions(select, '')
    expect(api.clearProgress).toHaveBeenCalledWith('movies', 603)
  })

  it('does not call the API when clearing an untracked title', async () => {
    renderTitle()
    const select = await screen.findByLabelText('Your status')
    await userEvent.selectOptions(select, '')
    expect(api.clearProgress).not.toHaveBeenCalled()
  })

  it('tracks season and episode for shows', async () => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 2, last_episode_num: 5 })
    vi.mocked(api.setProgress).mockResolvedValue({ status: 'watching', last_season_num: 3, last_episode_num: 1 })
    renderTitle('shows')
    const season = await screen.findByLabelText('Season')
    await waitFor(() => expect(season).toHaveValue(2))
    expect(screen.getByLabelText('Episode')).toHaveValue(5)
    await userEvent.clear(season)
    await userEvent.type(season, '3')
    await userEvent.clear(screen.getByLabelText('Episode'))
    await userEvent.type(screen.getByLabelText('Episode'), '1')
    await userEvent.click(screen.getByRole('button', { name: 'Save progress' }))
    expect(api.setProgress).toHaveBeenCalledWith('shows', 1396, { status: 'watching', last_season_num: 3, last_episode_num: 1 })
  })

  it('keeps season and episode when the status of a show changes', async () => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 2, last_episode_num: 5 })
    vi.mocked(api.setProgress).mockResolvedValue({ status: 'completed', last_season_num: 2, last_episode_num: 5 })
    renderTitle('shows')
    const select = await screen.findByLabelText('Your status')
    await waitFor(() => expect(select).toHaveValue('watching'))
    await userEvent.selectOptions(select, 'completed')
    expect(api.setProgress).toHaveBeenCalledWith('shows', 1396, { status: 'completed', last_season_num: 2, last_episode_num: 5 })
  })

  it('treats an empty episode as unset', async () => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 0, last_episode_num: 0 })
    vi.mocked(api.setProgress).mockResolvedValue({ status: 'watching', last_season_num: 0, last_episode_num: 0 })
    renderTitle('shows')
    const season = await screen.findByLabelText('Season')
    await waitFor(() => expect(screen.getByLabelText('Your status')).toHaveValue('watching'))
    await userEvent.clear(season)
    await userEvent.click(screen.getByRole('button', { name: 'Save progress' }))
    expect(api.setProgress).toHaveBeenLastCalledWith('shows', 1396, { status: 'watching', last_season_num: 0, last_episode_num: 0 })
    await userEvent.type(season, '2')
    await userEvent.clear(screen.getByLabelText('Episode'))
    await userEvent.click(screen.getByRole('button', { name: 'Save progress' }))
    expect(api.setProgress).toHaveBeenLastCalledWith('shows', 1396, { status: 'watching', last_season_num: 2, last_episode_num: 0 })
  })

  it('shows save errors', async () => {
    vi.mocked(api.setProgress).mockRejectedValue(new ApiError(400, 'bad_request', 'An episode needs a season'))
    renderTitle()
    await userEvent.selectOptions(await screen.findByLabelText('Your status'), 'dropped')
    expect(await screen.findByText('An episode needs a season')).toBeInTheDocument()
  })

  it('shows progress load errors', async () => {
    vi.mocked(api.getProgress).mockRejectedValue(new ApiError(500, 'internal', 'progress broke'))
    renderTitle()
    expect(await screen.findByText('progress broke')).toBeInTheDocument()
  })

  it('disables the controls while a save is in flight', async () => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 1, last_episode_num: 1 })
    vi.mocked(api.setProgress).mockReturnValue(new Promise(() => {}))
    renderTitle('shows')
    const select = await screen.findByLabelText('Your status')
    await waitFor(() => expect(select).toHaveValue('watching'))
    await userEvent.click(screen.getByRole('button', { name: 'Save progress' }))
    await waitFor(() => expect(select).toBeDisabled())
    expect(screen.getByRole('button', { name: 'Save progress' })).toBeDisabled()
  })

  it.each([['-1', '2'], ['2', '1.5']])('rejects season %s / episode %s without calling the API', async (season, episode) => {
    vi.mocked(api.getProgress).mockResolvedValue({ status: 'watching', last_season_num: 1, last_episode_num: 1 })
    renderTitle('shows')
    const seasonBox = await screen.findByLabelText('Season')
    await waitFor(() => expect(seasonBox).toHaveValue(1))
    await userEvent.clear(seasonBox)
    await userEvent.type(seasonBox, season)
    await userEvent.clear(screen.getByLabelText('Episode'))
    await userEvent.type(screen.getByLabelText('Episode'), episode)
    await userEvent.click(screen.getByRole('button', { name: 'Save progress' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/whole numbers/i)
    expect(api.setProgress).not.toHaveBeenCalled()
  })
})
