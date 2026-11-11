import { useCallback, useEffect, useRef, useState } from 'react'

export interface Polled<T> {
  /** Last successful result; kept while a later refresh fails. */
  data: T | null
  /** Error from the most recent refresh, if it failed. */
  error: string | null
  loading: boolean
  /** Fetch again now, e.g. right after changing something. */
  reload: () => void
}

/**
 * Calls `fetcher` every `intervalMs`, pausing while the tab is hidden. The
 * next call is scheduled after the previous one settles, so slow responses
 * never overlap.
 */
export function usePolling<T>(fetcher: (signal: AbortSignal) => Promise<T>, intervalMs = 5_000): Polled<T> {
  const [state, setState] = useState<Omit<Polled<T>, 'reload'>>({ data: null, error: null, loading: true })
  // Bumping this restarts the loop, which fetches immediately.
  const [generation, setGeneration] = useState(0)

  // Keep the latest fetcher without restarting the loop on every render.
  const fetcherRef = useRef(fetcher)
  useEffect(() => {
    fetcherRef.current = fetcher
  })

  useEffect(() => {
    const controller = new AbortController()
    let timer: number | undefined

    async function refresh() {
      if (!document.hidden) {
        try {
          const data = await fetcherRef.current(controller.signal)
          setState({ data, error: null, loading: false })
        } catch (err) {
          if (controller.signal.aborted) return
          const error = err instanceof Error ? err.message : String(err)
          setState((prev) => ({ data: prev.data, error, loading: false }))
        }
      }
      timer = window.setTimeout(refresh, intervalMs)
    }

    void refresh()
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [intervalMs, generation])

  const reload = useCallback(() => setGeneration((n) => n + 1), [])
  return { ...state, reload }
}
