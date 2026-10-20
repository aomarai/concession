import { useLayoutEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { springEasing, usePop } from '../lib/spring'

export interface Segment<V extends string> { value: V; label: ReactNode; name?: string }

const THUMB = springEasing()

function Option<V extends string>({ segment, checked, focusable, disabled, onChoose, onArrow }: {
  segment: Segment<V>
  checked: boolean
  focusable: boolean
  disabled?: boolean
  onChoose: () => void
  onArrow: (e: KeyboardEvent<HTMLButtonElement>) => void
}) {
  const pop = usePop<HTMLButtonElement>({ hover: false })
  return (
    <button
      {...pop}
      type="button" role="radio" aria-checked={checked} aria-label={segment.name} tabIndex={focusable ? 0 : -1} disabled={disabled}
      className="cn-seg__opt pop" onClick={onChoose}
      onKeyDown={(e) => { pop.onKeyDown?.(e); onArrow(e) }}
    >
      {segment.label}
    </button>
  )
}

// HIG segmented control: a radio group whose raised thumb springs to the choice.
export default function SegmentedControl<V extends string>({ label, segments, value, onChange, disabled }: {
  label: string
  segments: Segment<V>[]
  value: V
  onChange: (value: V) => void
  disabled?: boolean
}) {
  const track = useRef<HTMLDivElement>(null)
  const thumb = useRef<HTMLSpanElement>(null)
  const index = segments.findIndex((s) => s.value === value)

  useLayoutEffect(() => {
    const el = track.current?.querySelectorAll<HTMLElement>('[role="radio"]')[index]
    const t = thumb.current!
    t.style.opacity = el ? '1' : '0'
    if (!el) return
    t.style.width = `${el.offsetWidth}px`
    t.style.transform = `translateX(${el.offsetLeft}px)`
    // On a narrow screen the track scrolls: keep the choice in view without moving the page.
    track.current!.scrollLeft = el.offsetLeft - (track.current!.clientWidth - el.offsetWidth) / 2
  })

  function arrow(e: KeyboardEvent<HTMLButtonElement>, i: number) {
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key]
    if (!step) return
    e.preventDefault()
    const next = (i + step + segments.length) % segments.length
    onChange(segments[next].value)
    track.current!.querySelectorAll<HTMLElement>('[role="radio"]')[next].focus()
  }

  return (
    <div ref={track} role="radiogroup" aria-label={label} className="cn-seg">
      <span
        ref={thumb} aria-hidden="true" className="cn-seg__thumb"
        style={{ transition: `transform ${THUMB.duration}ms ${THUMB.easing}, width ${THUMB.duration}ms ${THUMB.easing}` }}
      />
      {segments.map((s, i) => (
        <Option
          key={s.value} segment={s} checked={i === index} focusable={i === index || (index < 0 && i === 0)} disabled={disabled}
          onChoose={() => { if (i !== index) onChange(s.value) }} onArrow={(e) => arrow(e, i)}
        />
      ))}
    </div>
  )
}
