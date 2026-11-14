import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { login, register } from '../api/endpoints'
import Alert from '../components/Alert'
import Brand from '../components/Brand'
import Button, { buttonClass } from '../components/Button'
import Poster from '../components/Poster'
import { errorMessage } from '../lib/format'
import { usePop } from '../lib/spring'

type Mode = 'signin' | 'register'

// The signed-out screen: sign in with a password, create an account, or go
// through Google. A successful sign-in sets the session cookie, so the only
// thing left to do is refetch /me — the app shell then renders.
export default function SignInPage() {
  const qc = useQueryClient()
  const pop = usePop<HTMLAnchorElement>({ entry: true, delay: 160 })
  const [mode, setMode] = useState<Mode>('signin')
  const [identifier, setIdentifier] = useState('')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')

  const signIn = useMutation({
    mutationFn: () => mode === 'signin'
      ? login(identifier.trim(), password)
      : register({ email: email.trim(), username: username.trim(), display_name: displayName.trim(), password }),
    onSuccess: () => qc.resetQueries(),
  })
  const switching = () => {
    setMode(mode === 'signin' ? 'register' : 'signin')
    signIn.reset()
  }

  return (
    <main className="mx-auto grid min-h-dvh max-w-[1280px] items-center gap-12 px-6 py-12 sm:px-8 lg:grid-cols-2">
      <div className="flex w-full max-w-md flex-col items-start gap-6">
        <p className="cn-nav__brand m-0"><Brand /></p>
        <h1 className="t-display-xl page-title">Movie night, sorted.</h1>
        <p className="t-body m-0 text-[18px] leading-7">Movie and TV watchlists to share with friends: plan what to watch, track where you are, and rate it after.</p>

        <form
          className="flex w-full flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            signIn.mutate()
          }}
        >
          {mode === 'signin' && (
            <label className="cn-field">
              E-mail or username
              <input
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                autoCapitalize="none"
                autoCorrect="off"
                autoComplete="username"
                className="cn-input"
              />
            </label>
          )}
          {mode === 'register' && (
            <>
              <label className="cn-field">
                E-mail
                <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoCapitalize="none" autoComplete="email" className="cn-input" />
              </label>
              <label className="cn-field">
                Username
                <input value={username} onChange={(e) => setUsername(e.target.value)} autoCapitalize="none" autoCorrect="off" className="cn-input" />
              </label>
              <label className="cn-field">
                Display name
                <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className="cn-input" />
              </label>
            </>
          )}
          <label className="cn-field">
            Password
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={mode === 'signin' ? 'current-password' : 'new-password'}
              className="cn-input"
            />
            {mode === 'register' && <span className="t-body-s text-ink-muted">At least 12 characters.</span>}
          </label>
          {signIn.isError && <Alert message={errorMessage(signIn.error)} />}
          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" disabled={signIn.isPending}>
              {mode === 'signin' ? 'Sign in' : 'Create account'}
            </Button>
            <Button variant="quiet" onClick={switching}>
              {mode === 'signin' ? 'Create an account' : 'I already have an account'}
            </Button>
          </div>
        </form>

        <p className="t-body-s m-0 text-ink-muted">Or</p>
        <a {...pop} href="/api/v1/auth/google/login" className={buttonClass('secondary')}>Sign in with Google</a>
      </div>
      <div className="cn-hero-cards hidden lg:block" aria-hidden="true">
        {['The Thing', 'Hereditary', 'Alien'].map((t) => <div key={t}><Poster title={t} size="fill" decorative /></div>)}
      </div>
    </main>
  )
}