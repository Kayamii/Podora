import { Field } from './ui'

interface Props {
  /** Every namespace present in the list being filtered. */
  namespaces: string[]
  value: string
  onChange: (namespace: string) => void
}

/** Picks one namespace or all. The selected one stays listed even if empty. */
export function NamespaceFilter({ namespaces, value, onChange }: Props) {
  const options = [...new Set([...namespaces, ...(value ? [value] : [])])].sort()
  return (
    <Field label="Namespace">
      <select className="input min-w-44" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">All namespaces</option>
        {options.map((ns) => (
          <option key={ns} value={ns}>
            {ns}
          </option>
        ))}
      </select>
    </Field>
  )
}

export function SearchField({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <Field label="Search">
      <input
        type="search"
        className="input w-64 max-w-full"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
    </Field>
  )
}
