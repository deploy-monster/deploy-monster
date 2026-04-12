import * as React from "react"

import { cn } from "@/lib/utils"

interface DropdownMenuContextValue {
  open: boolean
  setOpen: (open: boolean) => void
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
      if (e.key === "Escape") setOpen(false)
    }

    document.addEventListener("mousedown", handleClickOutside)
    document.addEventListener("keydown", handleEscape)
    return () => {
      document.removeEventListener("mousedown", handleClickOutside)
      document.removeEventListener("keydown", handleEscape)
    }
  }, [open])

  return (
    <DropdownMenuContext.Provider value={{ open, setOpen }}>
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
  const { open, setOpen } = useDropdownMenu()

  return (
    <button
      type="button"
      className={className}
      ref={ref}
      aria-expanded={open}
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
  const { open } = useDropdownMenu()

  if (!open) return null

  return (
    <div
      className={cn(
        "bg-popover text-popover-foreground absolute z-50 mt-1 min-w-[8rem] overflow-hidden rounded-md border p-1 shadow-md",
        "animate-in fade-in-0 zoom-in-95 duration-100",
        align === "start" && "left-0",
        align === "center" && "left-1/2 -translate-x-1/2",
        align === "end" && "right-0",
        className
      )}
      ref={ref}
      role="menu"
      {...props}
    />
  )
}

function DropdownMenuItem({
  className,
  inset,
  ref,
  ...props
}: React.ComponentProps<"button"> & {
  inset?: boolean
}) {
  const { setOpen } = useDropdownMenu()

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
        props.onClick?.(e)
        setOpen(false)
      }}
      {...props}
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
