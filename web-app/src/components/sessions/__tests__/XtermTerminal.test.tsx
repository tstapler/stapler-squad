import { render } from '@testing-library/react';
import { XtermTerminal } from '../XtermTerminal';

describe('XtermTerminal', () => {
  test('renders without error', () => {
    const { container } = render(<XtermTerminal />);
    expect(container.firstChild).toBeInTheDocument();
  });

  // mouseTracking prop removed (X.1): mouse tracking mode is set at runtime by PTY escape
  // sequences and read via terminal.modes.mouseTrackingMode — not configurable via prop.
  test('renders with only valid props', () => {
    const { container } = render(<XtermTerminal fontSize={14} scrollback={5000} />);
    expect(container.firstChild).toBeInTheDocument();
  });

  test('renders with default props', () => {
    const { container } = render(<XtermTerminal />);
    expect(container.firstChild).toBeInTheDocument();
  });
});
