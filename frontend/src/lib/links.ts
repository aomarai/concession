// Notification links come from the backend as API-style paths. Map the known
// ones to frontend routes and ignore anything else, so a link can never send
// the reader somewhere unexpected.
export function appPath(link?: string): string | undefined {
  if (link === '/invites' || link === '/friends') return link
  const m = link?.match(/^\/watchlists\/([\w-]+)$/)
  return m ? `/lists/${m[1]}` : undefined
}
