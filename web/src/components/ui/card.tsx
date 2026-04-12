import * as React from "react"

import { cn } from "@/lib/utils"

function Card({ className, ref, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "bg-card text-card-foreground flex flex-col gap-6 rounded-xl border py-6 shadow-sm",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function CardHeader({ className, ref, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "flex flex-col gap-1.5 px-6 @container/card-header",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function CardTitle({ className, ref, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("leading-none font-semibold", className)}
      ref={ref}
      {...props}
    />
  )
}

function CardDescription({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("text-muted-foreground text-sm", className)}
      ref={ref}
      {...props}
    />
  )
}

function CardContent({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div className={cn("px-6", className)} ref={ref} {...props} />
  )
}

function CardFooter({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("flex items-center px-6", className)}
      ref={ref}
      {...props}
    />
  )
}

export { Card, CardHeader, CardFooter, CardTitle, CardDescription, CardContent }
