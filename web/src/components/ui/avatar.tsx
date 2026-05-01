import * as React from "react"

import { cn } from "@/lib/utils"

function Avatar({
  className,
  ref,
  ...props
}: React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "relative flex size-8 shrink-0 overflow-hidden rounded-full",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function AvatarFallback({
  className,
  ref,
  ...props
}: React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "bg-muted flex size-full items-center justify-center rounded-full text-xs font-medium",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

export { Avatar, AvatarFallback }
