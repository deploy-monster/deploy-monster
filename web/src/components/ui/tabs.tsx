import * as React from "react"
import { useState, createContext, useContext } from "react"

import { cn } from "@/lib/utils"

interface TabsContextValue {
  value: string
  onValueChange: (value: string) => void
  /**
   * Stable id base derived from useId so triggers and panels can
   * declare matching ids per value (`${base}-trigger-${value}` and
   * `${base}-panel-${value}`) without forcing every caller to wire
   * aria-controls / aria-labelledby by hand.
   */
  baseId: string
}

const TabsContext = createContext<TabsContextValue>({
  value: "",
  onValueChange: () => {},
  baseId: "",
})

function triggerIdFor(base: string, value: string) {
  return `${base}-trigger-${value}`
}

function panelIdFor(base: string, value: string) {
  return `${base}-panel-${value}`
}

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
  const baseId = React.useId()

  return (
    <TabsContext.Provider value={{ value, onValueChange: handleValueChange, baseId }}>
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
  onKeyDown,
  ...props
}: React.ComponentProps<"button"> & { value: string }) {
  const context = useContext(TabsContext)
  const isActive = context.value === value

  // Arrow-key navigation moves the active tab; Home / End jump to
  // first / last. Matches the WAI-ARIA tablist pattern: the tablist
  // is one Tab stop, arrow keys move between tabs.
  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    onKeyDown?.(e)
    const tablist = e.currentTarget.parentElement
    if (!tablist) return
    const tabs = Array.from(
      tablist.querySelectorAll<HTMLButtonElement>('[role="tab"]:not([disabled])')
    )
    if (tabs.length === 0) return
    const idx = tabs.indexOf(e.currentTarget)
    if (idx < 0) return

    let next: HTMLButtonElement | null = null
    switch (e.key) {
      case "ArrowRight":
        next = tabs[(idx + 1) % tabs.length]
        break
      case "ArrowLeft":
        next = tabs[(idx - 1 + tabs.length) % tabs.length]
        break
      case "Home":
        next = tabs[0]
        break
      case "End":
        next = tabs[tabs.length - 1]
        break
    }
    if (next) {
      e.preventDefault()
      next.focus()
      // Clicking activates and updates context.value, which also
      // re-renders so the moved-to tab becomes tabIndex=0.
      next.click()
    }
  }

  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all outline-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        isActive
          ? "bg-background text-foreground shadow-sm"
          : "hover:text-foreground/80",
        className
      )}
      id={triggerIdFor(context.baseId, value)}
      role="tab"
      aria-selected={isActive}
      aria-controls={panelIdFor(context.baseId, value)}
      // Roving tabindex: only the active tab is in the tab order;
      // inactive tabs are reachable via arrow keys.
      tabIndex={isActive ? 0 : -1}
      onClick={() => context.onValueChange(value)}
      onKeyDown={handleKeyDown}
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
      id={panelIdFor(context.baseId, value)}
      role="tabpanel"
      aria-labelledby={triggerIdFor(context.baseId, value)}
      // Panel is focusable via Tab from the tablist so keyboard
      // users can move into the panel content. Inner focusable
      // children take precedence; tabIndex=0 just makes the panel
      // itself reachable when there are none.
      tabIndex={0}
      ref={ref}
      {...props}
    />
  )
}

export { Tabs, TabsList, TabsTrigger, TabsContent }
