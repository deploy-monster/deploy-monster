import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';

function Harness({ defaultValue = 'one' }: { defaultValue?: string }) {
  return (
    <Tabs defaultValue={defaultValue}>
      <TabsList>
        <TabsTrigger value="one">One</TabsTrigger>
        <TabsTrigger value="two">Two</TabsTrigger>
        <TabsTrigger value="three">Three</TabsTrigger>
      </TabsList>
      <TabsContent value="one">First panel</TabsContent>
      <TabsContent value="two">Second panel</TabsContent>
      <TabsContent value="three">Third panel</TabsContent>
    </Tabs>
  );
}

describe('Tabs a11y', () => {
  it('TabsList has role=tablist', () => {
    render(<Harness />);
    expect(screen.getByRole('tablist')).toBeInTheDocument();
  });

  it('TabsTrigger sets role=tab and aria-selected reflects active tab', () => {
    render(<Harness defaultValue="two" />);
    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(3);
    const two = screen.getByRole('tab', { name: 'Two' });
    expect(two).toHaveAttribute('aria-selected', 'true');
    const one = screen.getByRole('tab', { name: 'One' });
    expect(one).toHaveAttribute('aria-selected', 'false');
  });

  it('aria-controls on the trigger resolves to the rendered panel', () => {
    render(<Harness defaultValue="two" />);
    const two = screen.getByRole('tab', { name: 'Two' });
    const controls = two.getAttribute('aria-controls');
    expect(controls).toBeTruthy();
    const panel = document.getElementById(controls!);
    expect(panel).not.toBeNull();
    expect(panel).toHaveAttribute('role', 'tabpanel');
    expect(panel?.textContent).toBe('Second panel');
  });

  it('aria-labelledby on the panel points back to its trigger', () => {
    render(<Harness defaultValue="two" />);
    const panel = screen.getByRole('tabpanel');
    const labelledBy = panel.getAttribute('aria-labelledby');
    expect(labelledBy).toBeTruthy();
    const trigger = document.getElementById(labelledBy!);
    expect(trigger).toHaveAttribute('role', 'tab');
    expect(trigger?.textContent).toBe('Two');
  });

  it('roving tabindex: only the active trigger is in the tab order', () => {
    render(<Harness defaultValue="two" />);
    const one = screen.getByRole('tab', { name: 'One' });
    const two = screen.getByRole('tab', { name: 'Two' });
    const three = screen.getByRole('tab', { name: 'Three' });
    expect(one).toHaveAttribute('tabIndex', '-1');
    expect(two).toHaveAttribute('tabIndex', '0');
    expect(three).toHaveAttribute('tabIndex', '-1');
  });

  it('panel is itself focusable via tabIndex=0', () => {
    render(<Harness />);
    const panel = screen.getByRole('tabpanel');
    expect(panel).toHaveAttribute('tabIndex', '0');
  });

  it('ArrowRight moves focus and activation to next tab and wraps', () => {
    render(<Harness defaultValue="one" />);
    const one = screen.getByRole('tab', { name: 'One' });
    one.focus();
    expect(document.activeElement).toBe(one);

    fireEvent.keyDown(one, { key: 'ArrowRight' });
    const two = screen.getByRole('tab', { name: 'Two' });
    expect(document.activeElement).toBe(two);
    expect(two).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(two, { key: 'ArrowRight' });
    const three = screen.getByRole('tab', { name: 'Three' });
    expect(document.activeElement).toBe(three);

    fireEvent.keyDown(three, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(screen.getByRole('tab', { name: 'One' }));
  });

  it('ArrowLeft moves focus backwards and wraps', () => {
    render(<Harness defaultValue="one" />);
    const one = screen.getByRole('tab', { name: 'One' });
    one.focus();
    fireEvent.keyDown(one, { key: 'ArrowLeft' });
    expect(document.activeElement).toBe(screen.getByRole('tab', { name: 'Three' }));
  });

  it('Home / End jump to first / last tab', () => {
    render(<Harness defaultValue="two" />);
    const two = screen.getByRole('tab', { name: 'Two' });
    two.focus();

    fireEvent.keyDown(two, { key: 'End' });
    expect(document.activeElement).toBe(screen.getByRole('tab', { name: 'Three' }));

    fireEvent.keyDown(screen.getByRole('tab', { name: 'Three' }), { key: 'Home' });
    expect(document.activeElement).toBe(screen.getByRole('tab', { name: 'One' }));
  });

  it('clicking a trigger activates its panel', () => {
    render(<Harness defaultValue="one" />);
    expect(screen.getByRole('tabpanel').textContent).toBe('First panel');

    fireEvent.click(screen.getByRole('tab', { name: 'Three' }));
    expect(screen.getByRole('tabpanel').textContent).toBe('Third panel');
  });
});
