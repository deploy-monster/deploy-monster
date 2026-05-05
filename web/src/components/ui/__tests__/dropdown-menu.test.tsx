import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu';

function Harness({ onItem = () => {} }: { onItem?: (label: string) => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger>Open menu</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={() => onItem('first')}>First</DropdownMenuItem>
        <DropdownMenuItem onClick={() => onItem('second')}>Second</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => onItem('third')}>Third</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

describe('DropdownMenu a11y', () => {
  it('trigger exposes aria-haspopup="menu" and aria-expanded reflects state', () => {
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open menu' });
    expect(trigger).toHaveAttribute('aria-haspopup', 'menu');
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
  });

  it('aria-controls links the trigger to the content while open', () => {
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open menu' });
    expect(trigger).not.toHaveAttribute('aria-controls');
    fireEvent.click(trigger);
    const controls = trigger.getAttribute('aria-controls');
    expect(controls).toBeTruthy();
    const menu = document.getElementById(controls!);
    expect(menu).not.toBeNull();
    expect(menu).toHaveAttribute('role', 'menu');
  });

  it('separator carries role=separator and aria-orientation=horizontal', () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
    const sep = document.querySelector('[role="separator"]') as HTMLElement;
    expect(sep).not.toBeNull();
    expect(sep).toHaveAttribute('aria-orientation', 'horizontal');
  });

  it('focuses the first menuitem when opening', () => {
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open menu' });
    fireEvent.click(trigger);
    const first = screen.getByRole('menuitem', { name: 'First' });
    expect(document.activeElement).toBe(first);
  });

  it('ArrowDown moves focus to next menuitem and wraps at the end', () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));

    const menu = screen.getByRole('menu');
    const first = screen.getByRole('menuitem', { name: 'First' });
    const second = screen.getByRole('menuitem', { name: 'Second' });
    const third = screen.getByRole('menuitem', { name: 'Third' });

    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(second);
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(third);
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(first);
  });

  it('ArrowUp moves focus backward and wraps at the start', () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));

    const menu = screen.getByRole('menu');
    const first = screen.getByRole('menuitem', { name: 'First' });
    const third = screen.getByRole('menuitem', { name: 'Third' });

    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(third);
    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: 'Second' }));
    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(first);
  });

  it('Home and End jump to first / last menuitem', () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));

    const menu = screen.getByRole('menu');
    const first = screen.getByRole('menuitem', { name: 'First' });
    const third = screen.getByRole('menuitem', { name: 'Third' });

    fireEvent.keyDown(menu, { key: 'End' });
    expect(document.activeElement).toBe(third);
    fireEvent.keyDown(menu, { key: 'Home' });
    expect(document.activeElement).toBe(first);
  });

  it('Escape closes the menu and returns focus to the trigger', () => {
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open menu' });
    fireEvent.click(trigger);
    expect(screen.getByRole('menu')).toBeInTheDocument();

    // The Escape handler is wired at the document level; dispatch
    // there to mirror what a real keyboard event would do.
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(document.activeElement).toBe(trigger);
  });

  it('clicking a menuitem closes the menu and invokes its onClick', () => {
    const onItem = vi.fn();
    render(<Harness onItem={onItem} />);
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));

    fireEvent.click(screen.getByRole('menuitem', { name: 'Second' }));
    expect(onItem).toHaveBeenCalledWith('second');
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('clicking outside closes the menu without invoking any item', () => {
    const onItem = vi.fn();
    render(
      <div>
        <Harness onItem={onItem} />
        <span data-testid="outside">outside</span>
      </div>
    );
    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
    expect(screen.getByRole('menu')).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByTestId('outside'));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(onItem).not.toHaveBeenCalled();
  });
});
