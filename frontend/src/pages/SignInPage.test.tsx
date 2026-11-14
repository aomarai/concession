import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SignInPage from './SignInPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const me = { id: 'u1', username: 'ada', display_name: 'Ada L', avatar_url: '' }

describe('SignInPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('signs in with an e-mail or username and a password', async () => {
    vi.mocked(api.login).mockResolvedValue(me)
    renderWithProviders(<SignInPage />)

    await userEvent.type(screen.getByLabelText(/e-mail or username/i), ' Ada@example.com ')
    await userEvent.type(screen.getByLabelText(/password/i), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(api.login).toHaveBeenCalledWith('Ada@example.com', 'correct horse battery')
  })

  it('creates an account', async () => {
    vi.mocked(api.register).mockResolvedValue(me)
    renderWithProviders(<SignInPage />)

    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }))
    await userEvent.type(screen.getByLabelText(/e-mail$/i), ' ada@example.com ')
    await userEvent.type(screen.getByLabelText('Username'), ' ada.l ')
    await userEvent.type(screen.getByLabelText('Display name'), ' Ada L ')
    await userEvent.type(screen.getByLabelText(/password/i), 'correct horse battery')
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }))

    expect(api.register).toHaveBeenCalledWith({
      email: 'ada@example.com', username: 'ada.l', display_name: 'Ada L', password: 'correct horse battery',
    })
  })

  it('keeps the Google sign-in path', async () => {
    renderWithProviders(<SignInPage />)
    expect(screen.getByRole('link', { name: /sign in with google/i }))
      .toHaveAttribute('href', '/api/v1/auth/google/login')
  })

  it('shows the error from a failed sign-in and clears it when switching', async () => {
    vi.mocked(api.login).mockRejectedValue(new ApiError(401, 'unauthorized', 'Incorrect e-mail, username or password'))
    renderWithProviders(<SignInPage />)

    await userEvent.type(screen.getByLabelText(/e-mail or username/i), 'ada')
    await userEvent.type(screen.getByLabelText(/password/i), 'nope nope nope')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByText(/incorrect e-mail, username or password/i)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }))
    expect(screen.queryByText(/incorrect e-mail, username or password/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/e-mail$/i)).toBeInTheDocument()
  })

  it('goes back to signing in from the registration form', async () => {
    renderWithProviders(<SignInPage />)
    await userEvent.click(screen.getByRole('button', { name: 'Create an account' }))
    await userEvent.click(screen.getByRole('button', { name: 'I already have an account' }))
    expect(screen.getByLabelText(/e-mail or username/i)).toBeInTheDocument()
  })
})