import * as React from "react"
import { useEffect, useCallback } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
}

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
  return (
    <>
      <DialogOverlay onClick={onClose} />
      <div
        className={cn(
          "bg-background fixed left-1/2 top-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 border p-6 shadow-lg duration-200 sm:rounded-lg",
          className
        )}
        ref={ref}
        role="dialog"
        aria-modal="true"
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

function DialogDescription({
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

export {
  Dialog,
  DialogOverlay,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
}
