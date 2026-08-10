import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'

type VirtualListProps<T> = {
  items: T[]
  /** Fixed height, or a height computed from the item. Must not measure the
   *  DOM: heights are needed before a row renders. */
  rowHeight: number | ((item: T, index: number) => number)
  renderRow: (item: T, index: number) => ReactNode
  /** Key for React reconciliation; must be stable per item. */
  rowKey: (item: T, index: number) => string
  header?: ReactNode
  /** Fixed content width in px for horizontally scrolling grids. */
  contentWidth?: number
  className?: string
  overscan?: number
  empty?: ReactNode
}

/**
 * Virtual list with data-derived row heights.
 *
 * Rows are absolutely positioned at offsets taken from a prefix sum, so the
 * scroll height can never disagree with the rendered rows. Heights come from
 * the data rather than from measuring the DOM, which keeps the layout a pure
 * function of the input — the previous table-with-spacer-rows approach let the
 * two drift apart and that is what made the matrix shake while scrolling.
 */
export function VirtualList<T>({
  items,
  rowHeight,
  renderRow,
  rowKey,
  header,
  contentWidth,
  className,
  overscan = 6,
  empty,
}: VirtualListProps<T>) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const frame = useRef<number | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewport, setViewport] = useState(600)

  // offsets[i] is the top of row i; the last entry is the total height.
  const offsets = useMemo(() => {
    const result = new Array<number>(items.length + 1)
    result[0] = 0
    for (let i = 0; i < items.length; i++) {
      const height = typeof rowHeight === 'number' ? rowHeight : rowHeight(items[i], i)
      result[i + 1] = result[i] + height
    }
    return result
  }, [items, rowHeight])

  useLayoutEffect(() => {
    const element = scrollRef.current
    if (!element) return
    const measure = () => setViewport(element.clientHeight || 600)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    return () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current)
    }
  }, [])

  const onScroll = useCallback(() => {
    if (frame.current !== null) return
    frame.current = requestAnimationFrame(() => {
      frame.current = null
      setScrollTop(scrollRef.current?.scrollTop ?? 0)
    })
  }, [])

  const total = items.length
  const first = Math.max(0, indexAt(offsets, scrollTop) - overscan)
  const last = Math.min(total, indexAt(offsets, scrollTop + viewport) + 1 + overscan)

  const rows: ReactNode[] = []
  for (let i = first; i < last; i++) {
    rows.push(
      <div
        key={rowKey(items[i], i)}
        className="vrow"
        style={{ position: 'absolute', top: offsets[i], height: offsets[i + 1] - offsets[i], left: 0, right: 0 }}
      >
        {renderRow(items[i], i)}
      </div>,
    )
  }

  return (
    <div className={`virtual-scroll ${className ?? ''}`} ref={scrollRef} onScroll={onScroll}>
      <div className="virtual-inner" style={contentWidth ? { width: contentWidth } : undefined}>
        {header}
        {total === 0 ? (
          <div className="virtual-empty">{empty ?? '没有匹配的模型'}</div>
        ) : (
          <div className="virtual-body" style={{ height: offsets[total] }}>
            {rows}
          </div>
        )}
      </div>
    </div>
  )
}

/** Index of the row containing `position` (binary search over the prefix sum). */
function indexAt(offsets: number[], position: number): number {
  let low = 0
  let high = offsets.length - 2
  while (low < high) {
    const mid = (low + high + 1) >> 1
    if (offsets[mid] <= position) low = mid
    else high = mid - 1
  }
  return low
}
