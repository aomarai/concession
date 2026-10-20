import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe, inviteMember, listMembers, removeMember, rotateShareToken, setMemberRole } from '../api/endpoints'
import type { Member, MemberRole, WatchlistDetail } from '../api/types'
import { errorMessage } from '../lib/format'
import Alert from './Alert'
import Avatar from './Avatar'
import Button from './Button'

export default function SharingPanel({ list }: { list: WatchlistDetail }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const me = useQuery({ queryKey: ['me'], queryFn: getMe })
  const members = useQuery({ queryKey: ['members', list.id], queryFn: () => listMembers(list.id) })
  const [who, setWho] = useState('')
  const [role, setRole] = useState<MemberRole>('editor')
  const refresh = () => qc.invalidateQueries({ queryKey: ['members', list.id] })

  const invite = useMutation({
    mutationFn: () => inviteMember(list.id, who.trim(), role),
    onSuccess: () => { setWho(''); return refresh() },
  })
  const changeRole = useMutation({
    mutationFn: (v: { userId: string; role: MemberRole }) => setMemberRole(list.id, v.userId, v.role),
    onSuccess: refresh,
  })
  const remove = useMutation({ mutationFn: (userId: string) => removeMember(list.id, userId), onSuccess: refresh })
  const leave = useMutation({
    mutationFn: (userId: string) => removeMember(list.id, userId),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['watchlists'] })
      navigate('/')
    },
  })
  const rotate = useMutation({
    mutationFn: () => rotateShareToken(list.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['watchlist', list.id] }),
  })

  if (members.isError) return <Alert message={errorMessage(members.error)} />
  if (!members.data) return <p className="t-body text-ink-muted">Loading…</p>

  const isOwner = list.role === 'owner'
  const myId = me.data?.id
  const iAmMember = members.data.members.some((m) => m.id === myId)
  const actionError = [invite, changeRole, remove, leave, rotate].find((m) => m.isError)?.error

  function submit(e: FormEvent) {
    e.preventDefault()
    invite.mutate()
  }

  const row = (m: Member, pending = false) => (
    <li key={m.id} className="flex flex-wrap items-center gap-3">
      <Avatar name={m.display_name} src={m.avatar_url} decorative />
      <span className="t-label min-w-0 flex-1">{m.display_name}</span>
      {isOwner ? (
        <>
          <select
            aria-label={`Role for ${m.display_name}`} value={m.role}
            onChange={(e) => changeRole.mutate({ userId: m.id, role: e.target.value as MemberRole })}
            className="cn-select"
          >
            <option value="editor">Editor</option>
            <option value="viewer">Viewer</option>
          </select>
          <Button size="s" variant="danger" onClick={() => remove.mutate(m.id)} aria-label={pending ? `Cancel invite for ${m.display_name}` : `Remove ${m.display_name}`}>
            {pending ? 'Cancel invite' : 'Remove'}
          </Button>
        </>
      ) : (
        <span className="cn-tag">{m.role}</span>
      )}
    </li>
  )

  return (
    <section className="cn-panel">
      <h2 className="t-display-m m-0">Sharing</h2>
      <p className="t-body-s m-0 text-ink-muted">Owner: <span className="t-label text-ink">{members.data.owner.display_name}</span></p>
      <ul className="m-0 flex list-none flex-col gap-3 p-0">{members.data.members.map((m) => row(m))}</ul>
      {isOwner && members.data.pending && members.data.pending.length > 0 && (
        <>
          <h3 className="t-label m-0 text-ink-muted">Pending invitations</h3>
          <ul className="m-0 flex list-none flex-col gap-3 p-0">{members.data.pending.map((m) => row(m, true))}</ul>
        </>
      )}
      {isOwner && (
        <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
          <label className="cn-field min-w-0 flex-1">
            Username or e-mail
            <input value={who} onChange={(e) => setWho(e.target.value)} autoComplete="off" className="cn-input" />
          </label>
          <label className="cn-field">
            Role
            <select value={role} onChange={(e) => setRole(e.target.value as MemberRole)} className="cn-select">
              <option value="editor">Editor</option>
              <option value="viewer">Viewer</option>
            </select>
          </label>
          <Button type="submit" disabled={invite.isPending}>Invite</Button>
        </form>
      )}
      {isOwner && list.privacy === 'private' && (
        <p className="t-body-s m-0 text-ink-muted">Set the list to Shared or Public in List settings to get a share link.</p>
      )}
      {isOwner && list.privacy !== 'private' && list.share_token && (
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <label className="cn-field min-w-0 flex-1">
            Share link
            <input readOnly value={`${window.location.origin}/shared/${list.share_token}`} className="cn-input" />
          </label>
          <Button variant="secondary" onClick={() => rotate.mutate()} disabled={rotate.isPending}>New link</Button>
        </div>
      )}
      {!isOwner && iAmMember && myId && (
        <Button variant="danger" className="self-start" onClick={() => leave.mutate(myId)} disabled={leave.isPending}>Leave list</Button>
      )}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
    </section>
  )
}
