import * as React from "react"

import { cn } from "@/lib/utils"

function Tooltip({
  children,
  content,
  side = "top",
  className,
  delayMs = 0,
}: {
  children: React.ReactNode
  content: React.ReactNode
  side?: "top" | "bottom" | "left" | "right"
  className?: string
  delayMs?: number
}) {
  const [visible, setVisible] = React.useState(false)
  const timeoutRef = React.useRef<ReturnType<typeof setTimeout>>(null)

  const show = React.useCallback(() => {
    if (delayMs > 0) {
      timeoutRef.current = setTimeout(() => setVisible(true), delayMs)
    } else {
      setVisible(true)
    }
  }, [delayMs])

  const hide = React.useCallback(() => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    setVisible(false)
  }, [])

  // Promote string tooltips to an accessible name on the wrapped element
  // (typically an icon-only button). Skips any child that already exposes
  // its own aria-label or aria-labelledby.
  let accessibleChild: React.ReactNode = children
  if (
    typeof content === "string" &&
    React.isValidElement(children)
  ) {
    const existingProps = (children as React.ReactElement<{
      "aria-label"?: string
      "aria-labelledby"?: string
    }>).props
    if (!existingProps["aria-label"] && !existingProps["aria-labelledby"]) {
      accessibleChild = React.cloneElement(
        children as React.ReactElement<{ "aria-label"?: string }>,
        { "aria-label": content }
      )
    }
  }

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
    >
      {accessibleChild}
      {visible && (
        <div
          className={cn(
            "absolute z-50 w-max max-w-xs rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground shadow-md animate-in fade-in-0 zoom-in-95 duration-100",
            side === "top" && "bottom-full left-1/2 -translate-x-1/2 mb-2",
            side === "bottom" && "top-full left-1/2 -translate-x-1/2 mt-2",
            side === "left" && "right-full top-1/2 -translate-y-1/2 mr-2",
            side === "right" && "left-full top-1/2 -translate-y-1/2 ml-2",
            className
          )}
          role="tooltip"
        >
          {content}
        </div>
      )}
    </div>
  )
}

export { Tooltip }
