import * as React from "react"

import { cn } from "@/lib/utils"

function Label({
  className,
  ref,
  ...props
}: React.ComponentProps<"label">) {
  return (
    <label
      className={cn(
        "flex items-center gap-2 text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

export { Label }
