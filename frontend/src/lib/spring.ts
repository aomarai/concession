import { useCallback, type KeyboardEvent, type PointerEvent } from 'react'

// Spring physics behind Concession's "pop": touched things scale from 0.9,
// overshoot to 1.08 and settle at 1. Only `transform` is written, straight to
// the element (no React renders), so the work stays on the compositor.

export interface Spring { stiffness: number; damping: number; mass: number }

export const POP: Spring = { stiffness: 350, damping: 18, mass: 0.8 }
export const POP_FROM = 0.9
export const POP_PEAK = 1.08
const REST = 1
const STEP = 1 / 240 // integration step, seconds
const MAX_FRAME = 1 / 30 // a late frame is integrated as at most this long

// One semi-implicit Euler step of a damped spring pulling x toward target.
export function stepSpring(x: number, v: number, target: number, s: Spring, dt: number): [number, number] {
  const force = -s.stiffness * (x - target) - s.damping * v
  const nv = v + (force / s.mass) * dt
  return [x + nv * dt, nv]
}

const atRest = (x: number, v: number) => Math.abs(x - REST) < 1e-3 && Math.abs(v) < 1e-2

// Runs a spring from `from` with launch velocity v0 until it rests: its peak and how long it took.
export function simulate(from: number, target: number, v0: number, s: Spring): { peak: number; duration: number } {
  let x = from
  let v = v0
  let peak = from
  let t = 0
  while (t < 5 && !(t > 0 && atRest(x, v))) {
    ;[x, v] = stepSpring(x, v, target, s, STEP)
    peak = Math.max(peak, x)
    t += STEP
  }
  return { peak, duration: t }
}

// The launch velocity that makes the spring overshoot to `peak` before settling (bisection).
export function velocityForPeak(from: number, target: number, peak: number, s: Spring): number {
  let lo = 0
  let hi = 50
  for (let i = 0; i < 40; i++) {
    const mid = (lo + hi) / 2
    if (simulate(from, target, mid, s).peak < peak) lo = mid
    else hi = mid
  }
  return (lo + hi) / 2
}

const LAUNCH = velocityForPeak(POP_FROM, REST, POP_PEAK, POP)

// The POP spring as a CSS easing, for things CSS animates between two values
// (a segmented control's sliding thumb): `linear()` sampled from the simulation.
export function springEasing(s: Spring = POP, samples = 32): { easing: string; duration: number } {
  const { duration } = simulate(0, 1, 0, s)
  const points: string[] = []
  let x = 0
  let v = 0
  let t = 0
  for (let i = 1; i <= samples; i++) {
    const until = (duration * i) / samples
    while (t < until) {
      ;[x, v] = stepSpring(x, v, 1, s, STEP)
      t += STEP
    }
    points.push(i === samples ? '1' : x.toFixed(3))
  }
  return { easing: `linear(0, ${points.join(', ')})`, duration: Math.round(duration * 1000) }
}

export function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

const running = new WeakMap<HTMLElement, number>()

export function cancelPop(el: HTMLElement) {
  const id = running.get(el)
  if (id === undefined) return
  cancelAnimationFrame(id)
  running.delete(el)
}

// Pops one element: scale(0.9) → scale(1.08) → scale(1), on the POP spring.
export function popElement(el: HTMLElement) {
  cancelPop(el)
  if (prefersReducedMotion()) {
    el.style.transform = ''
    return
  }
  let x = POP_FROM
  let v = LAUNCH
  let last: number | undefined
  el.style.transform = `scale(${POP_FROM})`
  const tick = (now: number) => {
    let dt = last === undefined ? 1 / 60 : Math.min((now - last) / 1000, MAX_FRAME)
    last = now
    while (dt > 0) {
      const h = Math.min(dt, STEP)
      ;[x, v] = stepSpring(x, v, REST, POP, h)
      dt -= h
    }
    if (atRest(x, v)) {
      el.style.transform = ''
      running.delete(el)
      return
    }
    el.style.transform = `scale(${x.toFixed(4)})`
    running.set(el, requestAnimationFrame(tick))
  }
  running.set(el, requestAnimationFrame(tick))
}

export interface PopOptions {
  /** Pop when the element mounts (after `delay` ms, for staggering lists). */
  entry?: boolean
  delay?: number
  /** Pop when a mouse moves onto it. */
  hover?: boolean
  /** Pop on press: pointer down, or Enter/Space. */
  tap?: boolean
}

// Props to spread on the element that should pop. Give the element the `pop` class.
export function usePop<T extends HTMLElement>({ entry = false, delay = 0, hover = true, tap = true }: PopOptions = {}) {
  // A callback ref runs as the element mounts, before paint, so an entering element never
  // flashes at full size. React 19 calls a ref callback that returns a cleanup only with the
  // element, and runs the cleanup when it unmounts.
  const ref = useCallback((node: T | null) => {
    const el = node as T
    let timer: number | undefined
    if (entry && !prefersReducedMotion()) {
      el.style.transform = `scale(${POP_FROM})`
      timer = window.setTimeout(() => popElement(el), delay)
    }
    return () => {
      window.clearTimeout(timer)
      cancelPop(el)
    }
  }, [entry, delay])

  return {
    ref,
    onPointerEnter: hover ? (e: PointerEvent<T>) => { if (e.pointerType === 'mouse') popElement(e.currentTarget) } : undefined,
    onPointerDown: tap ? (e: PointerEvent<T>) => popElement(e.currentTarget) : undefined,
    onKeyDown: tap
      ? (e: KeyboardEvent<T>) => { if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) popElement(e.currentTarget) }
      : undefined,
  }
}
