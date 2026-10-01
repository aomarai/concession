import { ApiError } from '../api/client'
import { errorMessage, posterUrl, year } from './format'

it('errorMessage uses API messages and hides other errors', () => {
  expect(errorMessage(new ApiError(400, 'x', 'bad'))).toBe('bad')
  expect(errorMessage(new Error('secret internals'))).toMatch(/something went wrong/i)
})

it('posterUrl handles missing posters', () => {
  expect(posterUrl('/a.jpg')).toBe('https://image.tmdb.org/t/p/w92/a.jpg')
  expect(posterUrl('')).toBeUndefined()
  expect(posterUrl(undefined)).toBeUndefined()
})

it('year takes the first four characters', () => {
  expect(year('1999-03-30T00:00:00Z')).toBe('1999')
  expect(year('')).toBe('')
  expect(year(undefined)).toBe('')
})
