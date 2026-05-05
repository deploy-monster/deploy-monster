import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useState } from 'react';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetBody,
  SheetFooter,
} from '@/components/ui/sheet';

function Harness({ initialOpen = true }: { initialOpen?: boolean }) {
  const [open, setOpen] = useState(initialOpen);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open sheet</button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent onClose={() => setOpen(false)}>
          <SheetHeader>
            <SheetTitle>Edit profile</SheetTitle>
            <SheetDescription>Update your account settings.</SheetDescription>
          </SheetHeader>
          <SheetBody>
            <button>Save</button>
          </SheetBody>
          <SheetFooter>
            <button>Discard</button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </>
  );
}

describe('Sheet a11y', () => {
  it('renders nothing when closed', () => {
    render(<Harness initialOpen={false} />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('content carries role=dialog and aria-modal=true', () => {
    render(<Harness />);
    const sheet = screen.getByRole('dialog');
    expect(sheet).toHaveAttribute('aria-modal', 'true');
  });

  it('aria-labelledby and aria-describedby resolve to title and description', () => {
    render(<Harness />);
    const sheet = screen.getByRole('dialog');
    const labelledBy = sheet.getAttribute('aria-labelledby');
    const describedBy = sheet.getAttribute('aria-describedby');
    expect(document.getElementById(labelledBy!)?.textContent).toBe('Edit profile');
    expect(document.getElementById(describedBy!)?.textContent).toBe('Update your account settings.');
  });

  it('moves focus into the sheet container on open', () => {
    render(<Harness />);
    const sheet = screen.getByRole('dialog');
    expect(sheet).toHaveAttribute('tabIndex', '-1');
    expect(document.activeElement).toBe(sheet);
  });

  it('Escape closes the sheet', () => {
    render(<Harness />);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('Tab from the last focusable wraps to the first', () => {
    render(<Harness />);
    const sheet = screen.getByRole('dialog');
    const closeBtn = sheet.querySelector('button[aria-label="Close panel"]') as HTMLElement;
    const saveBtn = screen.getByRole('button', { name: 'Save' });

    closeBtn.focus();
    expect(document.activeElement).toBe(closeBtn);
    fireEvent.keyDown(sheet, { key: 'Tab' });
    expect(document.activeElement).toBe(saveBtn);
  });

  it('Shift+Tab from the container wraps to the last focusable (close button)', () => {
    render(<Harness />);
    const sheet = screen.getByRole('dialog');
    const closeBtn = sheet.querySelector('button[aria-label="Close panel"]') as HTMLElement;

    expect(document.activeElement).toBe(sheet);
    fireEvent.keyDown(sheet, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(closeBtn);
  });

  it('restores focus to the previously-active element on unmount', () => {
    const trigger = document.createElement('button');
    trigger.textContent = 'External';
    document.body.appendChild(trigger);
    trigger.focus();
    expect(document.activeElement).toBe(trigger);

    const { unmount } = render(<Harness />);
    expect(document.activeElement).toBe(screen.getByRole('dialog'));

    unmount();
    expect(document.activeElement).toBe(trigger);
    document.body.removeChild(trigger);
  });

  it('Close button has accessible name "Close panel"', () => {
    render(<Harness />);
    expect(screen.getByRole('button', { name: /close panel/i })).toBeInTheDocument();
  });

  it('Close button click invokes onOpenChange(false)', () => {
    const onOpenChange = vi.fn();
    render(
      <Sheet open onOpenChange={onOpenChange}>
        <SheetContent onClose={() => onOpenChange(false)}>
          <SheetHeader>
            <SheetTitle>T</SheetTitle>
            <SheetDescription>D</SheetDescription>
          </SheetHeader>
        </SheetContent>
      </Sheet>
    );
    fireEvent.click(screen.getByRole('button', { name: /close panel/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it('caller-supplied id on SheetTitle is preserved', () => {
    render(
      <Sheet open onOpenChange={() => {}}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle id="my-title">T</SheetTitle>
            <SheetDescription>D</SheetDescription>
          </SheetHeader>
        </SheetContent>
      </Sheet>
    );
    expect(document.getElementById('my-title')?.textContent).toBe('T');
  });
});
