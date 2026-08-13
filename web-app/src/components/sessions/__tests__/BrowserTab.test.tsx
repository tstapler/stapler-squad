import React from 'react';
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserTab, VNCStatus, buildWsUrl } from '../BrowserTab';
import type { VNCState } from '../BrowserTab';

// Helper to construct minimal VNCState objects for tests without requiring
// the full Message<> protobuf metadata at runtime.
function makeVncState(partial: Partial<VNCState>): VNCState {
  return partial as VNCState;
}

// Capture the latest onDisconnected callback so tests can simulate disconnects
let capturedOnDisconnected: (() => void) | undefined;
let capturedOnConnected: (() => void) | undefined;

// Mock CDPViewer — pure canvas component, no DOM APIs needed in tests
jest.mock('../CDPViewer', () => ({
  __esModule: true,
  default: jest.fn(({ wsUrl, onConnected, onDisconnected }: { wsUrl: string; onConnected?: () => void; onDisconnected?: () => void }) => {
    capturedOnConnected = onConnected;
    capturedOnDisconnected = onDisconnected;
    return <canvas data-testid="cdp-viewer" data-ws-url={wsUrl} />;
  }),
}));

// BrowserTab.css.ts uses vanilla-extract — mock the style module
jest.mock('../BrowserTab.css', () => ({
  browserTabContainer: 'browserTabContainer',
  viewerArea: 'viewerArea',
  placeholderOverlay: 'placeholderOverlay',
  canvasWrapper: 'canvasWrapper',
  reconnectingBanner: 'reconnectingBanner',
  reconnectButton: 'reconnectButton',
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

  it('BrowserTab_should_showReconnectingBanner_When_connectionDrops', () => {
    const readyState = makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true });
    render(<BrowserTab {...defaultProps} vncState={readyState} />);

    // Simulate the CDPViewer calling onDisconnected
    act(() => {
      capturedOnDisconnected?.();
    });

    expect(screen.getByText(/reconnecting/i)).toBeInTheDocument();
  });

  it('BrowserTab_should_hideReconnectingBanner_When_connectionRestored', () => {
    const readyState = makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true });
    render(<BrowserTab {...defaultProps} vncState={readyState} />);

    act(() => { capturedOnDisconnected?.(); });
    expect(screen.getByText(/reconnecting/i)).toBeInTheDocument();

    act(() => { capturedOnConnected?.(); });
    expect(screen.queryByText(/reconnecting/i)).not.toBeInTheDocument();
  });

  it('BrowserTab_should_remountViewer_When_reconnectButtonClicked', async () => {
    const user = userEvent.setup();
    const readyState = makeVncState({ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true });
    render(<BrowserTab {...defaultProps} vncState={readyState} />);

    act(() => { capturedOnDisconnected?.(); });
    expect(screen.getByText(/reconnecting/i)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /reconnect/i }));

    // Banner should be gone after manual reconnect
    expect(screen.queryByText(/reconnecting/i)).not.toBeInTheDocument();
  });

});

// ---------------------------------------------------------------------------
// buildWsUrl unit tests — covers the URL-hardening added in the Copilot review
// ---------------------------------------------------------------------------
describe('buildWsUrl', () => {
  const SESSION_ID = 'sess-42';

  // jsdom sets window.location to http://localhost by default.

  it('buildWsUrl_should_returnWss_When_httpsBaseUrl', () => {
    expect(buildWsUrl('https://example.com/api', SESSION_ID)).toBe(
      `wss://example.com/api/sessions/${SESSION_ID}/cdp-stream`
    );
  });

  it('buildWsUrl_should_returnWs_When_httpBaseUrl', () => {
    expect(buildWsUrl('http://example.com/api', SESSION_ID)).toBe(
      `ws://example.com/api/sessions/${SESSION_ID}/cdp-stream`
    );
  });

  it('buildWsUrl_should_stripTrailingSlash_When_baseUrlHasTrailingSlash', () => {
    const url = buildWsUrl('https://example.com/api/', SESSION_ID);
    expect(url).not.toContain('/api/api/');
    expect(url).toBe(`wss://example.com/api/sessions/${SESSION_ID}/cdp-stream`);
  });

  it('buildWsUrl_should_useWindowProtocol_When_protocolRelativeUrl', () => {
    // jsdom default protocol is http:
    const url = buildWsUrl('//example.com/api', SESSION_ID);
    // Should use ws:// because window.location.protocol is http: in jsdom
    expect(url).toMatch(/^ws:\/\//);
    expect(url).toBe(`ws://example.com/api/sessions/${SESSION_ID}/cdp-stream`);
  });

  it('buildWsUrl_should_useWindowOrigin_When_relativePathUrl', () => {
    // jsdom: window.location = http://localhost
    const url = buildWsUrl('/api', SESSION_ID);
    expect(url).toBe(`ws://localhost/api/sessions/${SESSION_ID}/cdp-stream`);
  });

  it('buildWsUrl_should_notDoubleSubstitute_When_wsUrlPassed', () => {
    const url = buildWsUrl('ws://example.com/api', SESSION_ID);
    expect(url).toMatch(/^ws:\/\//);
    expect(url).not.toMatch(/^wss:\/\//);
    expect(url).toBe(`ws://example.com/api/sessions/${SESSION_ID}/cdp-stream`);
  });

  it('buildWsUrl_should_notDoubleSubstitute_When_wssUrlPassed', () => {
    const url = buildWsUrl('wss://example.com/api', SESSION_ID);
    expect(url).toMatch(/^wss:\/\//);
    expect(url).toBe(`wss://example.com/api/sessions/${SESSION_ID}/cdp-stream`);
  });
});
