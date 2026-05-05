import * as React from "react"
import { useEffect, useCallback } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
}

// dialogIdsContext shares stable IDs from DialogContent down to
// DialogTitle/DialogDescription so the dialog container can name and
// describe itself via aria-labelledby/aria-describedby without forcing
// every caller to pass IDs through. Caller-supplied IDs still win.
const dialogIdsContext = React.createContext<{
  titleId: string
  descriptionId: string
} | null>(null)

// focusableSelector matches the elements that participate in keyboard
// tab navigation inside a dialog. Aligns with the WAI-ARIA dialog
// pattern guidance — anything tab-reachable in the surrounding page
// would be reachable in the dialog without this trap.
const focusableSelector =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function Dialog({ open, onOpenChange, children }: DialogProps) {
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

function DialogOverlay({
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

function DialogContent({
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

  // On mount: remember the element that had focus and move focus into
  // the dialog container so keyboard / screen-reader users land
  // inside the dialog. On unmount: restore focus to the original
  // element so closing the dialog returns the user where they were.
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

  // Focus trap: cycle Tab / Shift+Tab inside the dialog so the user
  // cannot tab back into the now-inert page behind the overlay. The
  // trap is intentionally permissive — it only prevents the wrap-
  // around when there are at least two focusable elements; a dialog
  // with a single button keeps the browser's natural behaviour.
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "Tab" || !containerRef.current) return
    const focusables = Array.from(
      containerRef.current.querySelectorAll<HTMLElement>(focusableSelector)
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
    <dialogIdsContext.Provider value={{ titleId, descriptionId }}>
      <DialogOverlay onClick={onClose} />
      <div
        className={cn(
          "bg-background fixed left-1/2 top-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 border p-6 shadow-lg duration-200 sm:rounded-lg",
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
            aria-label="Close dialog"
          >
            <X className="size-4" aria-hidden="true" />
            <span className="sr-only">Close</span>
          </button>
        )}
      </div>
    </dialogIdsContext.Provider>
  )
}

function DialogHeader({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 text-center sm:text-left",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function DialogFooter({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function DialogTitle({
  id,
  className,
  ref,
  ...props
}: React.ComponentProps<"h2">) {
  const ctx = React.useContext(dialogIdsContext)
  return (
    <h2
      id={id ?? ctx?.titleId}
      className={cn("text-lg font-semibold leading-none tracking-tight", className)}
      ref={ref}
      {...props}
    />
  )
}

function DialogDescription({
  id,
  className,
  ref,
  ...props
}: React.ComponentProps<"p">) {
  const ctx = React.useContext(dialogIdsContext)
  return (
    <p
      id={id ?? ctx?.descriptionId}
      className={cn("text-muted-foreground text-sm", className)}
      ref={ref}
      {...props}
    />
  )
}

export {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
}
