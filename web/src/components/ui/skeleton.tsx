import * as React from "react"

import { cn } from "@/lib/utils"

function Skeleton({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("bg-muted animate-pulse rounded-md", className)}
      ref={ref}
      {...props}
    />
  )
}

export { Skeleton }
