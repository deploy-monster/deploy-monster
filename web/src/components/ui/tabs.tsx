import * as React from "react"
import { useState, createContext, useContext } from "react"

import { cn } from "@/lib/utils"

interface TabsContextValue {
  value: string
  onValueChange: (value: string) => void
}

const TabsContext = createContext<TabsContextValue>({
  value: "",
  onValueChange: () => {},
})

function Tabs({
  className,
  defaultValue = "",
  value: controlledValue,
  onValueChange,
  children,
  ref,
  ...props
}: React.ComponentProps<"div"> & {
  defaultValue?: string
  value?: string
  onValueChange?: (value: string) => void
}) {
  const [uncontrolledValue, setUncontrolledValue] = useState(defaultValue)
  const value = controlledValue ?? uncontrolledValue
  const handleValueChange = onValueChange ?? setUncontrolledValue

  return (
    <TabsContext.Provider value={{ value, onValueChange: handleValueChange }}>
      <div
        className={cn("flex flex-col gap-2", className)}
        ref={ref}
        {...props}
      >
        {children}
      </div>
    </TabsContext.Provider>
  )
}

function TabsList({
  className,
  ref,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "bg-muted text-muted-foreground inline-flex h-9 w-fit items-center justify-center rounded-lg p-1",
        className
      )}
      role="tablist"
      ref={ref}
      {...props}
    />
  )
}

function TabsTrigger({
  className,
  value,
  ref,
  ...props
}: React.ComponentProps<"button"> & { value: string }) {
  const context = useContext(TabsContext)
  const isActive = context.value === value

  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all outline-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "hover:text-foreground/80",
        className
      )}
      role="tab"
      aria-selected={isActive}
      onClick={() => context.onValueChange(value)}
      ref={ref}
      {...props}
    />
  )
}

function TabsContent({
  className,
  value,
  ref,
  ...props
}: React.ComponentProps<"div"> & { value: string }) {
  const context = useContext(TabsContext)
  if (context.value !== value) return null

  return (
    <div
      className={cn("flex-1 outline-none", className)}
      role="tabpanel"
      ref={ref}
      {...props}
    />
  )
}

export { Tabs, TabsList, TabsTrigger, TabsContent }
