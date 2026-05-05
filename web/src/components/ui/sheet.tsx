import * as React from "react"
import { useEffect, useCallback } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

interface SheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
}

// sheetIdsContext mirrors dialogIdsContext: SheetTitle and
// SheetDescription register their auto-generated IDs so SheetContent
// can self-name and self-describe via aria-labelledby/aria-describedby
// without forcing every caller to hand-wire IDs.
const sheetIdsContext = React.createContext<{
  titleId: string
  descriptionId: string
} | null>(null)

const sheetFocusableSelector =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function Sheet({ open, onOpenChange, children }: SheetProps) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false)
    },
    [onOpenChange]
  )

  useEffect(() => {
    if (open) {
      document.addEventListener("keydown", handleKeyDown)
      document.body.style.overflow = "hidden"
      return () => {
        document.removeEventListener("keydown", handleKeyDown)
        document.body.style.overflow = ""
      }
    }
  }, [open, handleKeyDown])

  if (!open) return null

  return <>{children}</>
}

function SheetOverlay({
  className,
  onClick,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "fixed inset-0 z-50 bg-black/80",
        className
      )}
      onClick={onClick}
      ref={ref}
      {...props}
    />
  )
}

function SheetContent({
  className,
  children,
  onClose,
  ref,
  ...props
}: React.ComponentProps<"div"> & { onClose?: () => void }) {
  const titleId = React.useId()
  const descriptionId = React.useId()
  const containerRef = React.useRef<HTMLDivElement | null>(null)
  const previousActiveRef = React.useRef<Element | null>(null)

  // Same focus management as Dialog: capture the previously-active
  // element on mount, focus the container so AT lands inside the
  // sheet, restore focus to the original element on unmount.
  useEffect(() => {
    previousActiveRef.current = document.activeElement
    containerRef.current?.focus()
    return () => {
      const prev = previousActiveRef.current
      if (prev instanceof HTMLElement) {
        prev.focus()
      }
    }
  }, [])

  // Focus trap mirroring Dialog's: cycle Tab / Shift+Tab inside the
  // sheet so keyboard users can't tab into the inert page behind the
  // overlay.
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "Tab" || !containerRef.current) return
    const focusables = Array.from(
      containerRef.current.querySelectorAll<HTMLElement>(sheetFocusableSelector)
    ).filter((el) => !el.hasAttribute("disabled"))
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || active === containerRef.current)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }

  const setRef = (node: HTMLDivElement | null) => {
    containerRef.current = node
    if (typeof ref === "function") ref(node)
    else if (ref) (ref as React.MutableRefObject<HTMLDivElement | null>).current = node
  }

  return (
    <sheetIdsContext.Provider value={{ titleId, descriptionId }}>
      <SheetOverlay onClick={onClose} />
      <div
        className={cn(
          "bg-background fixed inset-y-0 right-0 z-50 ml-auto h-full w-full max-w-xl border-l shadow-xl",
          className
        )}
        ref={setRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        {...props}
      >
        {children}
        {onClose && (
          <button
            className="ring-offset-background absolute right-4 top-4 rounded-sm opacity-70 transition-opacity hover:opacity-100 focus:ring-2 focus:ring-offset-2 focus:outline-none disabled:pointer-events-none cursor-pointer"
            onClick={onClose}
            aria-label="Close panel"
          >
            <X className="size-4" aria-hidden="true" />
            <span className="sr-only">Close</span>
          </button>
        )}
      </div>
    </sheetIdsContext.Provider>
  )
}

function SheetHeader({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 border-b px-6 py-4",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function SheetFooter({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-col-reverse gap-2 border-t px-6 py-4 sm:flex-row sm:justify-end",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function SheetTitle({
  id,
  className,
  ref,
  ...props
}: React.ComponentProps<"h2">) {
  const ctx = React.useContext(sheetIdsContext)
  return (
    <h2
      id={id ?? ctx?.titleId}
      className={cn("text-lg font-semibold leading-none tracking-tight", className)}
      ref={ref}
      {...props}
    />
  )
}

function SheetDescription({
  id,
  className,
  ref,
  ...props
}: React.ComponentProps<"p">) {
  const ctx = React.useContext(sheetIdsContext)
  return (
    <p
      id={id ?? ctx?.descriptionId}
      className={cn("text-muted-foreground text-sm", className)}
      ref={ref}
      {...props}
    />
  )
}

function SheetBody({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex-1 overflow-y-auto px-6 py-4",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

export {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
  SheetBody,
}
