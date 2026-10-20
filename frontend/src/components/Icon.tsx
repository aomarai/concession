// Icons: 24px grid, 2px round-capped stroke in currentColor (design system Iconography).
const PATHS = {
  bookmark: 'M7 4h10a1 1 0 0 1 1 1v15l-6-4-6 4V5a1 1 0 0 1 1-1z',
  play: 'M8 5.5v13l10.5-6.5z',
  check: 'M5 12.5l4.5 4.5L19 7.5',
  cross: 'M7 7l10 10M17 7L7 17',
  search: 'M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM16.5 16.5L20 20',
  plus: 'M12 5v14M5 12h14',
  up: 'M5 15l7-7 7 7',
  down: 'M5 9l7 7 7-7',
  left: 'M15 5l-7 7 7 7',
  right: 'M9 5l7 7-7 7',
  alert: 'M12 3.5l9.5 16.5h-19zM12 10v4M12 17.2v.1',
  ticket: 'M4 7.5A2.5 2.5 0 0 1 6.5 5h11A2.5 2.5 0 0 1 20 7.5V10a2 2 0 0 0 0 4v2.5a2.5 2.5 0 0 1-2.5 2.5h-11A2.5 2.5 0 0 1 4 16.5V14a2 2 0 0 0 0-4zM14.5 5.5v2M14.5 11v2M14.5 16.5v2',
  calendar: 'M5.5 6h13a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1zM4.5 10.5h15M9 4v4M15 4v4',
  clock: 'M12 4a8 8 0 1 1 0 16 8 8 0 0 1 0-16zM12 8v4.5l3 2',
  film: 'M5 4.5h14a.5.5 0 0 1 .5.5v14a.5.5 0 0 1-.5.5H5a.5.5 0 0 1-.5-.5V5a.5.5 0 0 1 .5-.5zM8.5 4.5v15M15.5 4.5v15M4.5 9.5h4M4.5 14.5h4M15.5 9.5h4M15.5 14.5h4',
  tv: 'M4.5 7.5h15a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1zM8.5 3.5l3.5 4 3.5-4',
  note: 'M6 4.5h12a1 1 0 0 1 1 1v9.5l-4.5 4.5H6a1 1 0 0 1-1-1v-13a1 1 0 0 1 1-1zM8.5 9h7M8.5 12.5h4',
} as const

export type IconName = keyof typeof PATHS

export default function Icon({ name }: { name: IconName }) {
  return (
    <svg className="cn-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={PATHS[name]} />
    </svg>
  )
}
