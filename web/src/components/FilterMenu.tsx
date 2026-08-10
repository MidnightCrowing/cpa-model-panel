import { useEffect, useRef, useState } from 'react'

type FilterMenuProps = {
  label: string
  allLabel: string
  options: string[]
  selected: string[]
  onChange: (selected: string[]) => void
  /** Optional right-aligned annotation per option, e.g. a count. */
  note?: (option: string) => string | undefined
}

/** Multi-select dropdown that closes on outside click and on Escape. */
export function FilterMenu({ label, allLabel, options, selected, onChange, note }: FilterMenuProps) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const chosen = new Set(selected)
  const toggle = (option: string) => {
    const next = new Set(chosen)
    if (next.has(option)) next.delete(option)
    else next.add(option)
    onChange(options.filter((item) => next.has(item)))
  }

  return (
    <div className="filter-menu" ref={rootRef}>
      <button type="button" className={`filter-trigger ${open ? 'is-open' : ''}`} onClick={() => setOpen(!open)}>
        <span>{label}</span>
        <span className="filter-trigger-value">{selected.length ? `${selected.length} 项` : allLabel}</span>
      </button>
      {open && (
        <div className="filter-popover">
          <div className="filter-popover-actions">
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => onChange([])}>
              {allLabel}
            </button>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => onChange([...options])}>
              全选
            </button>
          </div>
          <div className="filter-options">
            {options.map((option) => (
              <label className="filter-option" key={option}>
                <input type="checkbox" checked={chosen.has(option)} onChange={() => toggle(option)} />
                <span className="filter-option-label">{option}</span>
                {note?.(option) && <span className="filter-option-note">{note(option)}</span>}
              </label>
            ))}
            {options.length === 0 && <div className="muted">无可选项</div>}
          </div>
        </div>
      )}
    </div>
  )
}
