import Button from './Button'

// Previous/Next for a paged list; renders nothing when there is only one page.
export default function Pager({ page, total, perPage, onChange }: { page: number; total: number; perPage: number; onChange: (page: number) => void }) {
  const pages = Math.ceil(total / perPage)
  if (pages <= 1) return null
  return (
    <nav className="cn-pager" aria-label="Pages">
      <Button variant="secondary" size="s" disabled={page <= 1} onClick={() => onChange(page - 1)}>Previous</Button>
      <span className="cn-pager__label">Page {page} of {pages}</span>
      <Button variant="secondary" size="s" disabled={page >= pages} onClick={() => onChange(page + 1)}>Next</Button>
    </nav>
  )
}
