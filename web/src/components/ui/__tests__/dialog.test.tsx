import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';

function Harness({ initialOpen = true }: { initialOpen?: boolean }) {
  const [open, setOpen] = useState(initialOpen);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent onClose={() => setOpen(false)}>
          <DialogHeader>
            <DialogTitle>Confirm action</DialogTitle>
            <DialogDescription>Are you sure?</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button>Cancel</button>
            <button>Confirm</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

describe('Dialog a11y', () => {
  it('does not render anything when closed', () => {
    render(<Harness initialOpen={false} />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('renders with role=dialog and aria-modal=true', () => {
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
  });

  it('wires aria-labelledby to the rendered DialogTitle', () => {
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    const labelledBy = dialog.getAttribute('aria-labelledby');
    expect(labelledBy).toBeTruthy();
    const titleEl = document.getElementById(labelledBy!);
    expect(titleEl).not.toBeNull();
    expect(titleEl?.textContent).toBe('Confirm action');
  });

  it('wires aria-describedby to the rendered DialogDescription', () => {
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    const describedBy = dialog.getAttribute('aria-describedby');
    expect(describedBy).toBeTruthy();
    const descEl = document.getElementById(describedBy!);
    expect(descEl).not.toBeNull();
    expect(descEl?.textContent).toBe('Are you sure?');
  });

  it('moves focus into the dialog container on open', () => {
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    // The container is tabIndex=-1 and is the auto-focus target so
    // screen-reader / keyboard users land inside the dialog rather
    // than wherever they were on the page.
    expect(dialog).toHaveAttribute('tabIndex', '-1');
    expect(document.activeElement).toBe(dialog);
  });

  it('closes when Escape is pressed', () => {
    render(<Harness />);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('cycles Tab from the last focusable back to the first', () => {
    // DOM order inside DialogContent is: {children} then the close
    // button, so the last focusable is the close button and the
    // first focusable is the first child button (Cancel here).
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    const closeBtn = dialog.querySelector('button[aria-label="Close dialog"]') as HTMLElement;
    const cancelBtn = screen.getByRole('button', { name: 'Cancel' });

    closeBtn.focus();
    expect(document.activeElement).toBe(closeBtn);

    fireEvent.keyDown(dialog, { key: 'Tab' });
    // Focus must wrap to the first focusable inside the dialog.
    expect(document.activeElement).toBe(cancelBtn);
  });

  it('Shift+Tab from the dialog container wraps to the last focusable', () => {
    render(<Harness />);
    const dialog = screen.getByRole('dialog');
    const closeBtn = dialog.querySelector('button[aria-label="Close dialog"]') as HTMLElement;

    // Initial mount focuses the dialog container itself; Shift+Tab
    // from there must land on the last focusable, which is the
    // DialogContent-supplied close button (rendered after children).
    expect(document.activeElement).toBe(dialog);
    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(closeBtn);
  });

  it('restores focus to the previously-active element on unmount', () => {
    const trigger = document.createElement('button');
    trigger.textContent = 'External trigger';
    document.body.appendChild(trigger);
    trigger.focus();
    expect(document.activeElement).toBe(trigger);

    const { unmount } = render(<Harness />);
    expect(document.activeElement).toBe(screen.getByRole('dialog'));

    unmount();
    expect(document.activeElement).toBe(trigger);
    document.body.removeChild(trigger);
  });

  it('Close button has an accessible name', () => {
    render(<Harness />);
    const closeBtn = screen.getByRole('button', { name: /close dialog/i });
    expect(closeBtn).toBeInTheDocument();
  });

  it('Close button click invokes onOpenChange(false)', () => {
    const onOpenChange = vi.fn();
    render(
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent onClose={() => onOpenChange(false)}>
          <DialogHeader>
            <DialogTitle>T</DialogTitle>
            <DialogDescription>D</DialogDescription>
          </DialogHeader>
        </DialogContent>
      </Dialog>
    );
    fireEvent.click(screen.getByRole('button', { name: /close dialog/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('caller-supplied id on DialogTitle takes precedence over the auto-generated one', () => {
    render(
      <Dialog open onOpenChange={() => {}}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle id="explicit-title">T</DialogTitle>
            <DialogDescription>D</DialogDescription>
          </DialogHeader>
        </DialogContent>
      </Dialog>
    );
    const title = document.getElementById('explicit-title');
    expect(title).not.toBeNull();
    // aria-labelledby still points to the auto-generated id, but a
    // caller passing an explicit id is honoured on the element itself
    // (matches shadcn convention; consumers manage the wiring).
    expect(title?.textContent).toBe('T');
  });
});
