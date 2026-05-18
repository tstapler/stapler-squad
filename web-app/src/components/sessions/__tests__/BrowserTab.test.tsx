import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { BrowserTab, VNCStatus } from '../BrowserTab';

// Mock CDPViewer — pure canvas component, no DOM APIs needed in tests
jest.mock('../CDPViewer', () => ({
  __esModule: true,
  default: jest.fn(({ wsUrl }: { wsUrl: string }) => (
    <canvas data-testid="cdp-viewer" data-ws-url={wsUrl} />
  )),
}));

// BrowserTab.css.ts uses vanilla-extract — mock the style module
jest.mock('../BrowserTab.css', () => ({
  browserTabContainer: 'browserTabContainer',
  qualityControls: 'qualityControls',
  qualityButton: 'qualityButton',
  qualityButtonActive: 'qualityButtonActive',
  qualityLabel: 'qualityLabel',
  viewerArea: 'viewerArea',
  placeholderOverlay: 'placeholderOverlay',
  canvasWrapper: 'canvasWrapper',
}));

describe('BrowserTab', () => {
  const defaultProps = {
    sessionId: 'test-session-id',
    baseUrl: 'http://localhost:8543/api',
    isVisible: true,
    vncState: undefined,
  };

  it('shows unavailable message when vncState is undefined', () => {
    render(<BrowserTab {...defaultProps} />);
    expect(screen.getByText(/unavailable on this host/i)).toBeInTheDocument();
  });

  it('shows unavailable message when status is UNAVAILABLE', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.UNAVAILABLE }} />);
    expect(screen.getByText(/unavailable on this host/i)).toBeInTheDocument();
  });

  it('shows "Starting" text when status is STARTING', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.STARTING }} />);
    expect(screen.getByText(/starting virtual display/i)).toBeInTheDocument();
  });

  it('shows "No browser open" when status is NO_BROWSER', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.NO_BROWSER }} />);
    expect(screen.getByText(/no browser open yet/i)).toBeInTheDocument();
  });

  it('does not mount CDPViewer until status is READY', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.NO_BROWSER }} />);
    expect(screen.queryByTestId('cdp-viewer')).not.toBeInTheDocument();
  });

  it('mounts CDPViewer when status becomes READY', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.READY, browserWindowDetected: true }} />);
    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();
  });

  it('builds correct ws:// URL from http:// baseUrl', () => {
    render(<BrowserTab {...defaultProps}
      baseUrl="http://localhost:8543/api"
      vncState={{ status: VNCStatus.READY, browserWindowDetected: true }}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).toBe('ws://localhost:8543/api/sessions/test-session-id/cdp-stream');
  });

  it('builds correct wss:// URL from https:// baseUrl', () => {
    render(<BrowserTab {...defaultProps}
      sessionId="test-session-id"
      baseUrl="https://myhost.example.com/api"
      vncState={{ status: VNCStatus.READY, browserWindowDetected: true }}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).toBe('wss://myhost.example.com/api/sessions/test-session-id/cdp-stream');
  });

  it('handles trailing slash in baseUrl without doubling /api', () => {
    render(<BrowserTab {...defaultProps}
      baseUrl="http://localhost:8543/api/"
      vncState={{ status: VNCStatus.READY, browserWindowDetected: true }}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).not.toContain('/api/api/');
  });

  it('shows quality controls when VNC is available', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.NO_BROWSER }} />);
    expect(screen.getByRole('button', { name: /medium/i })).toBeInTheDocument();
  });

  it('hides quality controls when VNC is unavailable', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.UNAVAILABLE }} />);
    expect(screen.queryByRole('button', { name: /medium/i })).not.toBeInTheDocument();
  });

  it('quality buttons have aria-pressed', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.NO_BROWSER }} />);
    const medBtn = screen.getByRole('button', { name: /medium/i });
    expect(medBtn).toHaveAttribute('aria-pressed', 'true'); // default is medium
    const lowBtn = screen.getByRole('button', { name: /low/i });
    expect(lowBtn).toHaveAttribute('aria-pressed', 'false');
  });

  it('quality button click updates aria-pressed', () => {
    render(<BrowserTab {...defaultProps} vncState={{ status: VNCStatus.NO_BROWSER }} />);
    fireEvent.click(screen.getByRole('button', { name: /low/i }));
    expect(screen.getByRole('button', { name: /low/i })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: /medium/i })).toHaveAttribute('aria-pressed', 'false');
  });
});
