/** The severity a log line was written at, as far as it can be told. */
export type LogLevel = 'error' | 'warn' | 'info' | 'debug'

// Each list covers the common shapes: plain words (ERROR), logfmt
// (level=error), JSON ("level":"error"), bracketed ([error], as nginx writes),
// and klog prefixes (E1110 12:00:00, as Kubernetes components write).
const ERROR: RegExp[] = [
  /\b(ERROR|ERR|FATAL|FATL|CRITICAL|CRIT|PANIC|EMERG|EMERGENCY|ALERT|SEVERE)\b/,
  /\b(level|lvl|severity)["']?\s*[=:]\s*["']?(error|err|fatal|panic|crit|critical|alert|emerg|emergency)\b/i,
  /\[(error|err|crit|critical|alert|emerg|fatal)\]/i,
  /^[EF]\d{4} \d/,
  /^(Error|error|Fatal|fatal):/,
  /^(panic: |Traceback \(most recent call last\)|Exception in thread )/,
  /\b[A-Z][A-Za-z0-9]*(Exception|Error)\b:/,
]

const WARN: RegExp[] = [
  /\b(WARN|WARNING|WRN)\b/,
  /\b(level|lvl|severity)["']?\s*[=:]\s*["']?(warn|warning)\b/i,
  /\[(warn|warning)\]/i,
  /^W\d{4} \d/,
  /^(Warning|warning):/,
]

const DEBUG: RegExp[] = [
  /\b(DEBUG|DBG|TRACE|TRC)\b/,
  /\b(level|lvl|severity)["']?\s*[=:]\s*["']?(debug|trace)\b/i,
  /\[(debug|trace)\]/i,
]

/** An RFC 3339 timestamp, as Kubernetes prefixes lines when asked to. */
const TIMESTAMP = /^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d))\s/

/** Lines that continue the entry above them: stack frames and wrapped text. */
const CONTINUATION = /^(\s+\S|at |Caused by:|\.\.\. \d+ more|goroutine \d+ \[)/

export interface ParsedLine {
  text: string
  /** The timestamp Kubernetes added, if timestamps are on. */
  timestamp: string | null
  level: LogLevel
}

/** Splits off the timestamp and works out each line's level. */
export function parseLines(lines: string[]): ParsedLine[] {
  const out: ParsedLine[] = []
  let previous: LogLevel = 'info'
  for (const line of lines) {
    const match = TIMESTAMP.exec(line)
    const text = match ? line.slice(match[0].length) : line
    let level = detect(text)
    // A stack frame under an error belongs to that error.
    if (level === 'info' && (previous === 'error' || previous === 'warn') && CONTINUATION.test(text)) {
      level = previous
    }
    out.push({ text, timestamp: match ? match[1] : null, level })
    previous = level
  }
  return out
}

function detect(text: string): LogLevel {
  if (ERROR.some((r) => r.test(text))) return 'error'
  if (WARN.some((r) => r.test(text))) return 'warn'
  if (DEBUG.some((r) => r.test(text))) return 'debug'
  return 'info'
}
