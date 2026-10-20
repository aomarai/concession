// A person's photo, or their initials on box-office teal.
// `decorative` when the name is already written beside it.
export default function Avatar({ name, src, size = 'm', decorative = false }: { name: string; src?: string; size?: 'm' | 'l'; decorative?: boolean }) {
  const initials = name.split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p.charAt(0)).join('').toUpperCase()
  return (
    <span className={size === 'l' ? 'cn-avatar cn-avatar--l' : 'cn-avatar'}>
      {src ? <img src={src} alt={decorative ? '' : name} /> : <span aria-hidden="true">{initials}</span>}
    </span>
  )
}
