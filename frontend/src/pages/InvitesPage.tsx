import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { acceptInvite, declineInvite, listInvites } from '../api/endpoints'
import Alert from '../components/Alert'
import Button from '../components/Button'
import PopRow from '../components/PopRow'
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
    <section className="flex flex-col gap-6">
      <h1 className="t-display-l page-title">Invitations</h1>
      {invites.isError && <Alert message={errorMessage(invites.error)} />}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {invites.data?.invites.length === 0 && <p className="t-body m-0 text-ink-muted">No pending invitations.</p>}
      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {invites.data?.invites.map((inv, i) => (
          <PopRow key={inv.id} index={i}>
            <div className="min-w-0 flex-1">
              <p className="t-title m-0">{inv.watchlist_title}</p>
              <p className="t-body-s m-0 text-ink-muted">
                {inv.invited_by ? `${inv.invited_by.display_name} invited you as ${inv.role}` : `Invited as ${inv.role}`}
              </p>
            </div>
            <Button onClick={() => accept.mutate(inv.id)}>Accept</Button>
            <Button variant="secondary" onClick={() => decline.mutate(inv.id)}>Decline</Button>
          </PopRow>
        ))}
      </ul>
    </section>
  )
}
