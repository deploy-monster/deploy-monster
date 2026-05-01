import * as React from "react"

import { cn } from "@/lib/utils"

function Table({ className, ref, ...props }: React.ComponentProps<"table">) {
  return (
    <div className="relative w-full overflow-auto">
      <table
        className={cn("w-full caption-bottom text-sm", className)}
        ref={ref}
        {...props}
      />
    </div>
  )
}

function TableHeader({
  className,
  ref,
  ...props
}: React.ComponentProps<"thead">) {
  return (
    <thead
      className={cn("[&_tr]:border-b", className)}
      ref={ref}
      {...props}
    />
  )
}

function TableBody({
  className,
  ref,
  ...props
}: React.ComponentProps<"tbody">) {
  return (
    <tbody
      className={cn("[&_tr:last-child]:border-0", className)}
      ref={ref}
      {...props}
    />
  )
}

function TableRow({
  className,
  ref,
  ...props
}: React.ComponentProps<"tr">) {
  return (
    <tr
      className={cn(
        "border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function TableHead({
  className,
  ref,
  ...props
}: React.ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "text-muted-foreground h-10 px-4 text-left align-middle font-medium whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

function TableCell({
  className,
  ref,
  ...props
}: React.ComponentProps<"td">) {
  return (
    <td
      className={cn(
        "p-4 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className
      )}
      ref={ref}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
}
