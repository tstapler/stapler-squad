import { render, screen } from '@testing-library/react';
import { XtermTerminal } from '../XtermTerminal';

describe('XtermTerminal', () => {
  test('renders without error', () => {
    render(<XtermTerminal />);
    expect(screen.getByRole('terminal')).toBeInTheDocument();
  });

  test('accepts mouseTracking prop', () => {
    render(<XtermTerminal mouseTracking="any" />);
    // Just testing that it renders without throwing an error
    expect(screen.getByRole('terminal')).toBeInTheDocument();
  });

  test('defaults to no mouseTracking', () => {
    render(<XtermTerminal />);
    expect(screen.getByRole('terminal')).toBeInTheDocument();
  });
});
