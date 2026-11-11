import { useEffect, useState } from 'react'
import { errorMessage } from '../services/api'

/** Lines kept in the browser; older ones are dropped while following. */
const MAX_LINES = 5000

export type StreamPhase = 'connecting' | 'open' | 'ended' | 'error'

export interface LogStream {
  lines: string[]
  phase: StreamPhase
  /** Set when phase is "error". */
  error: string | null
  /** True once older lines were dropped to stay within MAX_LINES. */
  truncated: boolean
}

/**
 * Reads a plain-text log response line by line as it arrives. For a followed
 * log the response never completes, so lines keep coming until the component
 * unmounts or the server closes the stream.
 *
 * Mount a fresh instance (change the `key`) to start a different stream.
 */
export function useLogStream(url: string): LogStream {
  const [state, setState] = useState<LogStream>({ lines: [], phase: 'connecting', error: null, truncated: false })

  useEffect(() => {
    const controller = new AbortController()

    async function read() {
      try {
        const res = await fetch(url, { signal: controller.signal })
        if (!res.ok || !res.body) {
          const error = res.ok ? 'The server sent no log data.' : await errorMessage(res)
          setState((s) => ({ ...s, phase: 'error', error }))
          return
        }
        setState((s) => ({ ...s, phase: 'open' }))

        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let partial = ''
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          const text = partial + decoder.decode(value, { stream: true })
          const pieces = text.split('\n')
          // The last piece is an unfinished line; keep it for the next chunk.
          partial = pieces.pop() ?? ''
          if (pieces.length > 0) append(pieces)
        }
        if (partial) append([partial])
        setState((s) => ({ ...s, phase: 'ended' }))
      } catch (err) {
        if (controller.signal.aborted) return
        const error = err instanceof Error ? err.message : String(err)
        setState((s) => ({ ...s, phase: 'error', error }))
      }
    }

    function append(incoming: string[]) {
      setState((s) => {
        const lines = s.lines.concat(incoming)
        const overflow = lines.length - MAX_LINES
        return overflow > 0
          ? { ...s, lines: lines.slice(overflow), truncated: true }
          : { ...s, lines }
      })
    }

    void read()
    return () => controller.abort()
  }, [url])

  return state
}
