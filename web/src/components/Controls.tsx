type SegmentedProps<T extends string> = {
  value: T
  options: Array<{ value: T; label: string; title?: string }>
  onChange: (value: T) => void
  label?: string
}

export function Segmented<T extends string>({ value, options, onChange, label }: SegmentedProps<T>) {
  return (
    <div className="segmented-control" role="group" aria-label={label}>
      {label && <span className="segmented-label">{label}</span>}
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          title={option.title}
          className={`segmented-item ${value === option.value ? 'is-active' : ''}`}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

type CheckboxProps = {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  title?: string
}

export function Checkbox({ checked, onChange, label, title }: CheckboxProps) {
  return (
    <label className="checkbox" title={title}>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span>{label}</span>
    </label>
  )
}
