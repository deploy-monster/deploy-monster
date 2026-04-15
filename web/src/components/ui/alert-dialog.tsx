import * as React from "react"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "./dialog"
import { Button } from "./button"

interface AlertDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  variant?: "default" | "destructive"
  onConfirm: () => void
  loading?: boolean
}

export function AlertDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  variant = "default",
  onConfirm,
  loading = false,
}: AlertDialogProps) {
  const handleConfirm = () => {
    onConfirm()
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onClose={() => onOpenChange(false)}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            {cancelLabel}
          </Button>
          <Button
            variant={variant === "destructive" ? "destructive" : "default"}
            onClick={handleConfirm}
            disabled={loading}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function useConfirmDialog() {
  const [state, setState] = React.useState<{
    open: boolean
    title: string
    description: string
    variant: "default" | "destructive"
    onConfirm: () => void
  }>({ open: false, title: "", description: "", variant: "default", onConfirm: () => {} })

  const confirm = React.useCallback((opts: {
    title: string
    description: string
    variant?: "default" | "destructive"
    onConfirm: () => void
  }) => {
    setState({
      open: true,
      title: opts.title,
      description: opts.description,
      variant: opts.variant ?? "default",
      onConfirm: opts.onConfirm,
    })
  }, [])

  const cancel = React.useCallback(() => {
    setState((s) => ({ ...s, open: false }))
  }, [])

  const dialog = (
    <AlertDialog
      open={state.open}
      onOpenChange={(open) => {
        if (!open) cancel()
      }}
      title={state.title}
      description={state.description}
      variant={state.variant}
      onConfirm={state.onConfirm}
    />
  )

  return { confirm, cancel, dialog }
}
