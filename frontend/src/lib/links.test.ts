import { appPath } from './links'

describe('appPath', () => {
  it.each([
    ['/watchlists/6f1c0b0e-1111-4222-8333-444455556666', '/lists/6f1c0b0e-1111-4222-8333-444455556666'],
    ['/invites', '/invites'],
    ['/friends', '/friends'],
  ])('maps %s', (input, expected) => expect(appPath(input)).toBe(expected))

  it.each([undefined, '', 'https://evil.example/x', '//evil.example', '/watchlists/a/b', '/other'])(
    'ignores %s',
    (input) => expect(appPath(input)).toBeUndefined(),
  )
})
