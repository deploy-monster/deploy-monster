import * as React from "react"

import { cn } from "@/lib/utils"

interface DropdownMenuContextValue {
  open: boolean
  setOpen: (open: boolean) => void
  contentId: string
  triggerRef: React.MutableRefObject<HTMLButtonElement | null>
  /**
   * closeAndReturnFocus closes the menu and moves focus back to the
   * trigger button. Used by the Escape handler so keyboard users
   * end up where they started rather than at the top of the page.
   */
  closeAndReturnFocus: () => void
}

const DropdownMenuContext = React.createContext<DropdownMenuContextValue | null>(
  null
)

function useDropdownMenu() {
  const ctx = React.useContext(DropdownMenuContext)
  if (!ctx)
    throw new Error(
      "DropdownMenu components must be used within <DropdownMenu>"
    )
  return ctx
}

function DropdownMenu({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false)
  const containerRef = React.useRef<HTMLDivElement>(null)
  const triggerRef = React.useRef<HTMLButtonElement | null>(null)
  const contentId = React.useId()

  const closeAndReturnFocus = React.useCallback(() => {
    setOpen(false)
    triggerRef.current?.focus()
  }, [])

  React.useEffect(() => {
    if (!open) return

    function handleClickOutside(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false)
      }
    }

    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") closeAndReturnFocus()
    }

    document.addEventListener("mousedown", handleClickOutside)
    document.addEventListener("keydown", handleEscape)
    return () => {
      document.removeEventListener("mousedown", handleClickOutside)
      document.removeEventListener("keydown", handleEscape)
    }
  }, [open, closeAndReturnFocus])

  return (
    <DropdownMenuContext.Provider
      value={{ open, setOpen, contentId, triggerRef, closeAndReturnFocus }}
    >
      <div ref={containerRef} className="relative inline-block">
        {children}
      </div>
    </DropdownMenuContext.Provider>
  )
}

function DropdownMenuTrigger({
  className,
  ref,
  ...props
}: React.ComponentProps<"button">) {
  const { open, setOpen, contentId, triggerRef } = useDropdownMenu()

  const setRef = (node: HTMLButtonElement | null) => {
    triggerRef.current = node
    if (typeof ref === "function") ref(node)
    else if (ref) (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node
  }

  return (
    <button
      type="button"
      className={className}
      ref={setRef}
      aria-haspopup="menu"
      aria-expanded={open}
      aria-controls={open ? contentId : undefined}
      onClick={() => setOpen(!open)}
      {...props}
    />
  )
}

function DropdownMenuContent({
  className,
  align = "start",
  ref,
  ...props
}: React.ComponentProps<"div"> & {
  align?: "start" | "center" | "end"
}) {
  const { open, contentId, closeAndReturnFocus } = useDropdownMenu()
  const contentRef = React.useRef<HTMLDivElement | null>(null)

  // When the menu opens, move focus to the first menuitem so keyboard
  // users land inside the menu and arrow-key navigation works
  // immediately. The WAI-ARIA menu pattern requires this; without it
  // focus stays on the trigger and ArrowDown does nothing useful.
  React.useEffect(() => {
    if (!open) return
    const first = contentRef.current?.querySelector<HTMLElement>(
      '[role="menuitem"]:not([disabled])'
    )
    first?.focus()
  }, [open])

  const setRef = (node: HTMLDivElement | null) => {
    contentRef.current = node
    if (typeof ref === "function") ref(node)
    else if (ref) (ref as React.MutableRefObject<HTMLDivElement | null>).current = node
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!contentRef.current) return
    const items = Array.from(
      contentRef.current.querySelectorAll<HTMLElement>(
        '[role="menuitem"]'
      )
    ).filter((el) => !el.hasAttribute("disabled"))
    if (items.length === 0) return

    const currentIdx = items.indexOf(document.activeElement as HTMLElement)
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault()
        items[(currentIdx + 1 + items.length) % items.length].focus()
        break
      case "ArrowUp":
        e.preventDefault()
        items[(currentIdx - 1 + items.length) % items.length].focus()
        break
      case "Home":
        e.preventDefault()
        items[0].focus()
        break
      case "End":
        e.preventDefault()
        items[items.length - 1].focus()
        break
      case "Escape":
        // Escape on the content also lands here; close and return
        // focus to the trigger so the user is back where they
        // started instead of stranded on a now-removed element.
        e.preventDefault()
        closeAndReturnFocus()
        break
    }
  }

  if (!open) return null

  return (
    <div
      id={contentId}
      className={cn(
        "bg-popover text-popover-foreground absolute z-50 mt-1 min-w-[8rem] overflow-hidden rounded-md border p-1 shadow-md",
        "animate-in fade-in-0 zoom-in-95 duration-100",
        align === "start" && "left-0",
        align === "center" && "left-1/2 -translate-x-1/2",
        align === "end" && "right-0",
        className
      )}
      ref={setRef}
      role="menu"
      onKeyDown={handleKeyDown}
      {...props}
    />
  )
}

function DropdownMenuItem({
  className,
  inset,
  ref,
  onClick,
  ...rest
}: React.ComponentProps<"button"> & {
  inset?: boolean
}) {
  const { setOpen } = useDropdownMenu()

  // Destructure onClick so the close-on-click wrapper isn't shadowed
  // by a later {...props} spread — the previous version used
  // onClick={...} {...props} which silently dropped the setOpen call
  // for any caller who passed an onClick handler. Bug fix.
  return (
    <button
      type="button"
      className={cn(
        "relative flex w-full cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
        inset && "pl-8",
        className
      )}
      ref={ref}
      role="menuitem"
      onClick={(e) => {
        onClick?.(e)
        setOpen(false)
      }}
      {...rest}
    />
  )
}

function DropdownMenuSeparator({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("bg-border -mx-1 my-1 h-px", className)}
      ref={ref}
      role="separator"
      aria-orientation="horizontal"
      {...props}
    />
  )
}

function DropdownMenuLabel({
  className,
  inset,
  ref,
  ...props
}: React.ComponentProps<"div"> & {
  inset?: boolean
}) {
  return (
    <div
      className={cn(
        "px-2 py-1.5 text-sm font-semibold",
        inset && "pl-8",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
}
