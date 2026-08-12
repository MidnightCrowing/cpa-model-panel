import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

/** Half the tooltip's max width, used to keep it inside the viewport. */
const HALF_WIDTH = 168

/**
 * A hover tooltip the page draws itself.
 *
 * The native `title` attribute cannot be laid out or coloured and the browser
 * decides when it appears, so the matrix draws its own: a two-column body with
 * the model on the left and its outcome on the right.
 *
 * It appears immediately and never takes the pointer (`pointer-events: none`),
 * flipping above the anchor in the lower half of the screen and clamping to the
 * viewport horizontally, so it cannot sit between you and what you are reading.
 *
 * Rendered in a portal: a cell inside the virtualised, clipped grid has to be
 * able to paint outside it.
 */
export function Tooltip({
  content,
  children,
  className = '',
}: {
  content: ReactNode
  children: ReactNode
  className?: string
}) {
  const anchorRef = useRef<HTMLSpanElement>(null)
  const [placement, setPlacement] = useState<{ left: number; top: number; above: boolean } | null>(null)

  const hide = useCallback(() => setPlacement(null), [])

  // Measured against the viewport: the anchor sits inside a scroller, so a
  // fixed-position box measured at show time is the only stable answer.
  const show = useCallback(() => {
    const rect = anchorRef.current?.getBoundingClientRect()
    if (!rect) return
    const above = rect.top > window.innerHeight / 2
    const centre = rect.left + rect.width / 2
    const limit = Math.max(HALF_WIDTH, window.innerWidth - HALF_WIDTH)
    setPlacement({
      left: Math.min(Math.max(centre, HALF_WIDTH), limit),
      top: above ? rect.top - 8 : rect.bottom + 8,
      above,
    })
  }, [])

  // Any scroll invalidates the measured position, and the grid scrolls a lot.
  useEffect(() => {
    if (!placement) return
    window.addEventListener('scroll', hide, true)
    window.addEventListener('resize', hide)
    return () => {
      window.removeEventListener('scroll', hide, true)
      window.removeEventListener('resize', hide)
    }
  }, [hide, placement])

  return (
    <>
      <span
        ref={anchorRef}
        className={className}
        onPointerEnter={show}
        onPointerLeave={hide}
        onPointerDown={hide}
      >
        {children}
      </span>
      {placement &&
        createPortal(
          <div
            role="tooltip"
            className={`tooltip ${placement.above ? 'is-above' : 'is-below'}`}
            style={{ left: placement.left, top: placement.top }}
          >
            {content}
          </div>,
          document.body,
        )}
    </>
  )
}
