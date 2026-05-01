import * as React from "react"
import { useEffect, useCallback } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

interface SheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
}

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
  return (
    <>
      <SheetOverlay onClick={onClose} />
      <div
        className={cn(
          "bg-background fixed inset-y-0 right-0 z-50 ml-auto h-full w-full max-w-xl border-l shadow-xl",
          className
        )}
        ref={ref}
        {...props}
      >
        {children}
        {onClose && (
          <button
            className="ring-offset-background absolute right-4 top-4 rounded-sm opacity-70 transition-opacity hover:opacity-100 focus:ring-2 focus:ring-offset-2 focus:outline-none disabled:pointer-events-none cursor-pointer"
            onClick={onClose}
          >
            <X className="size-4" />
            <span className="sr-only">Close</span>
          </button>
        )}
      </div>
    </>
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
  className,
  ref,
  ...props
}: React.ComponentProps<"h2">) {
  return (
    <h2
      className={cn("text-lg font-semibold leading-none tracking-tight", className)}
      ref={ref}
      {...props}
    />
  )
}

function SheetDescription({
  className,
  ref,
  ...props
}: React.ComponentProps<"p">) {
  return (
    <p
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
