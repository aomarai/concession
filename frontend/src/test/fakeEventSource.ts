type Listener = (e: MessageEvent) => void

// Minimal stand-in for the browser's EventSource so tests can push events.
export class FakeEventSource {
  static instances: FakeEventSource[] = []
  url: string
  withCredentials: boolean
  closed = false
  private listeners = new Map<string, Listener[]>()

  constructor(url: string, init?: { withCredentials?: boolean }) {
    this.url = url
    this.withCredentials = init?.withCredentials ?? false
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, fn: Listener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }

  close() {
    this.closed = true
  }

  emit(type: string, data: unknown = {}) {
    for (const fn of this.listeners.get(type) ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data) }))
  }

  static reset() {
    FakeEventSource.instances = []
  }

  static get last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]
  }
}
