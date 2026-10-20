const buttonClass = 'rounded bg-zinc-800 px-3 py-1 text-sm disabled:opacity-50'

// Previous/Next for a paged list; renders nothing when there is only one page.
export default function Pager({ page, total, perPage, onChange }: { page: number; total: number; perPage: number; onChange: (page: number) => void }) {
  const pages = Math.ceil(total / perPage)
  if (pages <= 1) return null
  return (
    <div className="flex items-center gap-2">
      <button disabled={page <= 1} onClick={() => onChange(page - 1)} className={buttonClass}>Previous</button>
      <span className="text-sm text-zinc-400">Page {page} of {pages}</span>
      <button disabled={page >= pages} onClick={() => onChange(page + 1)} className={buttonClass}>Next</button>
    </div>
  )
}
