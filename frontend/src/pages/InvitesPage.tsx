import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { acceptInvite, declineInvite, listInvites } from '../api/endpoints'
import Alert from '../components/Alert'
import { errorMessage } from '../lib/format'

export default function InvitesPage() {
  const qc = useQueryClient()
  const invites = useQuery({ queryKey: ['invites'], queryFn: listInvites })
  const done = () => Promise.all([
    qc.invalidateQueries({ queryKey: ['invites'] }),
    qc.invalidateQueries({ queryKey: ['watchlists'] }),
  ])
  const accept = useMutation({ mutationFn: (id: string) => acceptInvite(id), onSuccess: done })
  const decline = useMutation({ mutationFn: (id: string) => declineInvite(id), onSuccess: done })
  const actionError = [accept, decline].find((m) => m.isError)?.error

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">Invitations</h1>
      {invites.isError && <Alert message={errorMessage(invites.error)} />}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {invites.data?.invites.length === 0 && <p className="text-zinc-400">No pending invitations.</p>}
      <ul className="space-y-3">
        {invites.data?.invites.map((inv) => (
          <li key={inv.id} className="flex items-center gap-3 rounded bg-zinc-900 p-3">
            <div className="flex-1">
              <p className="font-medium">{inv.watchlist_title}</p>
              <p className="text-sm text-zinc-400">
                {inv.invited_by ? `${inv.invited_by.display_name} invited you as ${inv.role}` : `Invited as ${inv.role}`}
              </p>
            </div>
            <button onClick={() => accept.mutate(inv.id)} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900">Accept</button>
            <button onClick={() => decline.mutate(inv.id)} className="rounded bg-zinc-800 px-3 py-1 text-sm">Decline</button>
          </li>
        ))}
      </ul>
    </section>
  )
}
