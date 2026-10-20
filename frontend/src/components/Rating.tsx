import { useRef, useState, type KeyboardEvent } from 'react'
import { popElement, usePop } from '../lib/spring'

const SEATS = Array.from({ length: 10 }, (_, i) => i + 1)

// A rating out of 10 as a row of cinema seats, read-only.
export function RatingDisplay({ value, size = 's' }: { value?: number; size?: 's' | 'm' }) {
  return (
    <span className={`cn-rating cn-rating--${size}`} role="img" aria-label={value ? `Rated ${value} out of 10` : 'Not rated'}>
      <span className="cn-rating__seats">
        {SEATS.map((n) => <span key={n} className={value && n <= value ? 'cn-seat is-on' : 'cn-seat'} />)}
      </span>
      {value ? <span className="cn-rating__score">{value}</span> : <span className="cn-rating__none">Not rated</span>}
    </span>
  )
}

// The score as a glass chip, for laying over a poster.
export function RatingChip({ value }: { value: number }) {
  return (
    <span className="cn-glass" role="img" aria-label={`Rated ${value} out of 10`}>
      <span className="cn-seat" />{value}
    </span>
  )
}

function Seat({ n, on, checked, focusable, onPick, onHover, onKey }: {
  n: number; on: boolean; checked: boolean; focusable: boolean
  onPick: () => void; onHover: () => void; onKey: (e: KeyboardEvent<HTMLButtonElement>) => void
}) {
  const pop = usePop<HTMLButtonElement>({ tap: false })
  return (
    <button
      {...pop}
      type="button" role="radio" aria-checked={checked} aria-label={`${n} out of 10`} tabIndex={focusable ? 0 : -1}
      className={on ? 'cn-seat pop is-on' : 'cn-seat pop'} onClick={onPick} onMouseEnter={onHover} onKeyDown={onKey}
    />
  )
}

// The rating input: ten seats. Choosing fills them left to right, each popping 20ms after the last.
export function RatingInput({ label, value, onChange }: { label: string; value: number; onChange: (n: number) => void }) {
  const [hover, setHover] = useState(0)
  const seats = useRef<HTMLSpanElement>(null)
  const shown = hover || value

  function choose(n: number) {
    onChange(n)
    seats.current!.querySelectorAll<HTMLElement>('[role="radio"]').forEach((el, i) => {
      if (i < n) window.setTimeout(() => popElement(el), i * 20)
    })
  }

  function key(e: KeyboardEvent<HTMLButtonElement>) {
    const step = { ArrowRight: 1, ArrowUp: 1, ArrowLeft: -1, ArrowDown: -1 }[e.key]
    if (!step) return
    e.preventDefault()
    const next = Math.min(10, Math.max(1, value + step))
    choose(next)
    seats.current!.querySelectorAll<HTMLElement>('[role="radio"]')[next - 1].focus()
  }

  return (
    <span className="cn-rating cn-rating--l">
      <span ref={seats} className="cn-rating__seats" role="radiogroup" aria-label={label} onMouseLeave={() => setHover(0)}>
        {SEATS.map((n) => (
          <Seat
            key={n} n={n} on={n <= shown} checked={n === value} focusable={n === (value || 1)}
            onPick={() => choose(n)} onHover={() => setHover(n)} onKey={key}
          />
        ))}
      </span>
      {value ? <span className="cn-rating__score" aria-hidden="true">{value}</span> : <span className="cn-rating__none">Choose a seat to rate</span>}
    </span>
  )
}
