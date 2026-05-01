import * as React from "react"

import { cn } from "@/lib/utils"

function ScrollArea({
  className,
  children,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "relative overflow-auto",
        "[&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar]:h-2",
        "[&::-webkit-scrollbar-track]:bg-transparent",
        "[&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-border",
        "[&::-webkit-scrollbar-thumb:hover]:bg-muted-foreground/30",
        "scrollbar-thin scrollbar-track-transparent scrollbar-thumb-border",
        className
      )}
      ref={ref}
      {...props}
    >
      {children}
    </div>
  )
}

export { ScrollArea }
