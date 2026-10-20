// The Concession lockup: the app-icon symbol (a ticket with its stub tearing off, from
// design/brand/concession-app-icon.svg) drawn in theme tokens, and the name.
export function BrandMark() {
  return (
    <svg className="cn-nav__tile" viewBox="0 0 512 512" aria-hidden="true">
      <rect width="512" height="512" rx="114" fill="var(--velvet)" />
      <path fill="var(--on-velvet)" d="M87.838 223.71C83.618 199.779 99.598 176.958 123.529 172.738L290.947 143.218 327.413 350.028 159.995 379.548C136.064 383.768 113.243 367.788 109.023 343.857L104.335 317.267C122.827 314.006 135.175 296.372 131.914 277.88 128.654 259.387 111.019 247.04 92.527 250.3Z" />
      <path fill="var(--butter)" d="M339.81 349.242 328.819 139.529 374.756 137.122C399.023 135.85 419.727 154.492 420.999 178.759L422.412 205.722C403.66 206.705 389.255 222.703 390.238 241.455 391.221 260.207 407.219 274.611 425.971 273.629L427.384 300.592C428.656 324.859 410.014 345.562 385.747 346.834Z" />
    </svg>
  )
}

export default function Brand() {
  return (
    <>
      <BrandMark />
      <span>Concession</span>
    </>
  )
}
