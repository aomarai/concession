import { ApiError } from '../api/client'

export function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Something went wrong. Please try again.'
}

export function posterUrl(path?: string): string | undefined {
  return path ? `https://image.tmdb.org/t/p/w92${path}` : undefined
}

export function year(date?: string): string {
  return date ? date.slice(0, 4) : ''
}
