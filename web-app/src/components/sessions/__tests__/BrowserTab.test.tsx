import React from 'react';
import { render, screen } from '@testing-library/react';
import { BrowserTab, VNCStatus } from '../BrowserTab';
import type { VNCState } from '../BrowserTab';

// Helper to construct minimal VNCState objects for tests without requiring
// the full Message<> protobuf metadata at runtime.
function makeVncState(partial: Partial<VNCState>): VNCState {
  return partial as VNCState;
}

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
    render(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_UNAVAILABLE })} />);
    expect(screen.getByText(/unavailable on this host/i)).toBeInTheDocument();
  });

  it('shows "Starting" text when status is STARTING', () => {
    render(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_STARTING })} />);
    expect(screen.getByText(/starting virtual display/i)).toBeInTheDocument();
  });

  it('shows "No browser open" when status is NO_BROWSER', () => {
    render(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_NO_BROWSER })} />);
    expect(screen.getByText(/no browser open yet/i)).toBeInTheDocument();
  });

  it('does not mount CDPViewer until status is READY', () => {
    render(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_NO_BROWSER })} />);
    expect(screen.queryByTestId('cdp-viewer')).not.toBeInTheDocument();
  });

  it('mounts CDPViewer when status becomes READY', () => {
    render(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true })} />);
    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();
  });

  it('builds correct ws:// URL from http:// baseUrl', () => {
    render(<BrowserTab {...defaultProps}
      baseUrl="http://localhost:8543/api"
      vncState={makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true })}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).toBe('ws://localhost:8543/api/sessions/test-session-id/cdp-stream');
  });

  it('builds correct wss:// URL from https:// baseUrl', () => {
    render(<BrowserTab {...defaultProps}
      sessionId="test-session-id"
      baseUrl="https://myhost.example.com/api"
      vncState={makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true })}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).toBe('wss://myhost.example.com/api/sessions/test-session-id/cdp-stream');
  });

  it('handles trailing slash in baseUrl without doubling /api', () => {
    render(<BrowserTab {...defaultProps}
      baseUrl="http://localhost:8543/api/"
      vncState={makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true })}
    />);
    const viewer = screen.getByTestId('cdp-viewer');
    expect(viewer.getAttribute('data-ws-url')).not.toContain('/api/api/');
  });

  it('BrowserTab_should_keepViewerMounted_When_vncBecomesUnavailableAfterReady', () => {
    // Start in READY state so hasBeenReadyRef is set to true
    const readyState = makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true });
    const { rerender } = render(<BrowserTab {...defaultProps} vncState={readyState} />);

    // Confirm viewer is mounted in the ready state
    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();

    // Transition to STARTING (temporarily unavailable)
    rerender(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_STARTING })} />);

    // CDPViewer should remain mounted (sticky-mount behavior via hasBeenReadyRef)
    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();
  });

  it('BrowserTab_should_keepViewerMounted_When_vncBecomesNoBrowserAfterReady', () => {
    const readyState = makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true });
    const { rerender } = render(<BrowserTab {...defaultProps} vncState={readyState} />);
    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();

    rerender(<BrowserTab {...defaultProps} vncState={makeVncState({ status: VNCStatus.VNC_STATUS_NO_BROWSER })} />);

    expect(screen.getByTestId('cdp-viewer')).toBeInTheDocument();
  });

});
