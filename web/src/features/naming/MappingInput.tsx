import { memo, useEffect, useState } from 'react'

type MappingInputProps = {
  /** Row identity: changing it resets the local buffer. */
  rowKey: string
  value: string
  placeholder?: string
  disabled?: boolean
  dirty?: boolean
  onCommit: (value: string) => void
}

/**
 * Text field that keeps the keystrokes local and only reports on blur/Enter.
 *
 * Hundreds of these exist at once; dispatching per keystroke is what made
 * typing lag in the previous version.
 */
export const MappingInput = memo(function MappingInput({
  rowKey,
  value,
  placeholder,
  disabled,
  dirty,
  onCommit,
}: MappingInputProps) {
  const [local, setLocal] = useState(value)
  const [editing, setEditing] = useState(false)

  useEffect(() => {
    if (!editing) setLocal(value)
  }, [editing, value, rowKey])

  return (
    <input
      className={`input mono name-input ${dirty ? 'is-dirty' : ''}`}
      value={local}
      disabled={disabled}
      placeholder={placeholder}
      spellCheck={false}
      onFocus={() => setEditing(true)}
      onChange={(event) => setLocal(event.target.value)}
      onBlur={() => {
        setEditing(false)
        if (local !== value) onCommit(local)
      }}
      onKeyDown={(event) => {
        if (event.key === 'Enter') event.currentTarget.blur()
        if (event.key === 'Escape') {
          setLocal(value)
          setEditing(false)
          event.currentTarget.blur()
        }
      }}
    />
  )
})
