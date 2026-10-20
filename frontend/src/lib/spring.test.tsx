import { act, fireEvent, render, screen } from '@testing-library/react'
import {
  POP, POP_FROM, POP_PEAK, cancelPop, popElement, prefersReducedMotion, simulate, springEasing, stepSpring, usePop, velocityForPeak,
} from './spring'

// Drive requestAnimationFrame by hand: each pump() runs the queued callbacks at the next timestamp.
let queue: Map<number, FrameRequestCallback>
let nextId: number
let now: number
function pump(ms = 16) {
  now += ms
  const due = [...queue]
  queue.clear()
  for (const [, cb] of due) cb(now)
}
function pumpUntilIdle(limit = 400) {
  let n = 0
  while (queue.size > 0 && n++ < limit) pump()
  return n
}

function setReducedMotion(reduce: boolean | undefined) {
  if (reduce === undefined) {
    vi.stubGlobal('matchMedia', undefined)
    return
  }
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: reduce && q.includes('reduce'), media: q }))
}

beforeEach(() => {
  queue = new Map()
  nextId = 1
  now = 0
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => { const id = nextId++; queue.set(id, cb); return id })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => { queue.delete(id) })
  setReducedMotion(false)
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('spring physics', () => {
  it('steps a damped spring toward its target', () => {
    const [x, v] = stepSpring(0.9, 0, 1, POP, 1 / 240)
    expect(v).toBeGreaterThan(0)
    expect(x).toBeGreaterThan(0.9)
    expect(x).toBeLessThan(1)
  })

  it('finds the launch velocity that overshoots to the requested peak, then settles', () => {
    const v0 = velocityForPeak(POP_FROM, 1, POP_PEAK, POP)
    const run = simulate(POP_FROM, 1, v0, POP)
    expect(run.peak).toBeCloseTo(POP_PEAK, 3)
    expect(run.duration).toBeGreaterThan(0.2)
    expect(run.duration).toBeLessThan(2)
  })

  it('exposes the spring as a CSS linear() easing that overshoots and ends at 1', () => {
    const { easing, duration } = springEasing(POP, 8)
    expect(easing).toMatch(/^linear\(0, [\d., ]+, 1\)$/)
    const values = easing.slice(7, -1).split(', ').map(Number)
    expect(values).toHaveLength(9)
    expect(Math.max(...values)).toBeGreaterThan(1)
    expect(duration).toBeGreaterThan(200)
    expect(springEasing().easing.split(',')).toHaveLength(33)
  })

  it('uses the stiffness 350, damping 18, mass 0.8 spring', () => {
    expect(POP).toEqual({ stiffness: 350, damping: 18, mass: 0.8 })
  })
})

describe('prefersReducedMotion', () => {
  it('follows the media query, and is false where matchMedia is missing', () => {
    expect(prefersReducedMotion()).toBe(false)
    setReducedMotion(true)
    expect(prefersReducedMotion()).toBe(true)
    setReducedMotion(undefined)
    expect(prefersReducedMotion()).toBe(false)
  })
})

describe('popElement', () => {
  it('scales from 0.9 past 1.08 and back to rest, writing only transform', () => {
    const el = document.createElement('div')
    popElement(el)
    expect(el.style.transform).toBe('scale(0.9)')
    let max = 0
    let frames = 0
    while (queue.size > 0 && frames++ < 400) {
      pump()
      const m = /scale\(([\d.]+)\)/.exec(el.style.transform)
      if (m) max = Math.max(max, Number(m[1]))
    }
    expect(max).toBeGreaterThan(1.07)
    expect(max).toBeLessThan(1.09)
    expect(el.style.transform).toBe('')
    expect(frames).toBeLessThan(150)
  })

  it('keeps the motion stable when frames arrive late', () => {
    const el = document.createElement('div')
    popElement(el)
    pump(500)
    expect(Number(/scale\(([\d.]+)\)/.exec(el.style.transform)?.[1])).toBeLessThan(1.2)
    pumpUntilIdle()
    expect(el.style.transform).toBe('')
  })

  it('restarts instead of stacking when popped again mid-flight', () => {
    const el = document.createElement('div')
    popElement(el)
    pump()
    popElement(el)
    expect(queue.size).toBe(1)
    expect(el.style.transform).toBe('scale(0.9)')
  })

  it('does nothing visible when reduced motion is preferred', () => {
    setReducedMotion(true)
    const el = document.createElement('div')
    el.style.transform = 'scale(0.9)'
    popElement(el)
    expect(el.style.transform).toBe('')
    expect(queue.size).toBe(0)
  })

  it('cancelPop stops a running pop and ignores idle elements', () => {
    const el = document.createElement('div')
    cancelPop(el)
    popElement(el)
    cancelPop(el)
    expect(queue.size).toBe(0)
  })
})

function Card({ entry, delay, hover, tap }: { entry?: boolean; delay?: number; hover?: boolean; tap?: boolean }) {
  const pop = usePop<HTMLButtonElement>({ entry, delay, hover, tap })
  return <button {...pop}>Card</button>
}

describe('usePop', () => {
  it('pops on entry after its delay', () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    render(<Card entry delay={40} />)
    const el = screen.getByRole('button')
    expect(el.style.transform).toBe('scale(0.9)')
    expect(queue.size).toBe(0)
    act(() => { vi.advanceTimersByTime(40) })
    expect(queue.size).toBe(1)
    pumpUntilIdle()
    expect(el.style.transform).toBe('')
  })

  it('does not set an entry scale under reduced motion', () => {
    setReducedMotion(true)
    render(<Card entry />)
    expect(screen.getByRole('button').style.transform).toBe('')
  })

  it('pops on mouse hover but not on touch hover', () => {
    render(<Card />)
    const el = screen.getByRole('button')
    fireEvent.pointerEnter(el, { pointerType: 'touch' })
    expect(queue.size).toBe(0)
    fireEvent.pointerEnter(el, { pointerType: 'mouse' })
    expect(queue.size).toBe(1)
  })

  it('pops on tap and on keyboard activation', () => {
    render(<Card />)
    const el = screen.getByRole('button')
    fireEvent.pointerDown(el)
    expect(el.style.transform).toBe('scale(0.9)')
    pumpUntilIdle()
    fireEvent.keyDown(el, { key: 'a' })
    fireEvent.keyDown(el, { key: 'Enter', repeat: true })
    expect(queue.size).toBe(0)
    fireEvent.keyDown(el, { key: ' ' })
    expect(queue.size).toBe(1)
    pumpUntilIdle()
    fireEvent.keyDown(el, { key: 'Enter' })
    expect(queue.size).toBe(1)
  })

  it('can opt out of hover and tap', () => {
    render(<Card hover={false} tap={false} />)
    const el = screen.getByRole('button')
    fireEvent.pointerEnter(el, { pointerType: 'mouse' })
    fireEvent.pointerDown(el)
    fireEvent.keyDown(el, { key: 'Enter' })
    expect(queue.size).toBe(0)
  })

  it('is harmless when its ref is never attached', () => {
    function Detached() {
      usePop<HTMLDivElement>({ entry: true })
      return <p>Plain</p>
    }
    const { unmount } = render(<Detached />)
    unmount()
    expect(queue.size).toBe(0)
  })

  it('cancels a pending entry and a running pop on unmount', () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const { unmount } = render(<Card entry delay={100} />)
    unmount()
    act(() => { vi.advanceTimersByTime(100) })
    expect(queue.size).toBe(0)

    const second = render(<Card />)
    fireEvent.pointerDown(screen.getByRole('button'))
    expect(queue.size).toBe(1)
    second.unmount()
    expect(queue.size).toBe(0)
  })
})
