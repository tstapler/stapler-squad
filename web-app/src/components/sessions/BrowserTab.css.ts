import { style } from '@vanilla-extract/css';
import { vars } from '../../styles/theme-contract.css';

export const browserTabContainer = style({
  display: 'flex',
  flexDirection: 'column',
  height: '100%',
  position: 'relative',
  overflow: 'hidden',
});

export const viewerArea = style({
  flex: 1,
  position: 'relative',
  overflow: 'hidden',
  minHeight: 0,
});

export const placeholderOverlay = style({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  height: '100%',
  flexDirection: 'column',
  gap: vars.space['2'],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  textAlign: 'center',
  padding: vars.space['4'],
});

export const canvasWrapper = style({
  position: 'absolute',
  inset: 0,
});

export const reconnectingBanner = style({
  position: 'absolute',
  top: vars.space['2'],
  left: '50%',
  transform: 'translateX(-50%)',
  zIndex: 10,
  display: 'flex',
  alignItems: 'center',
  gap: vars.space['2'],
  padding: `${vars.space['1']} ${vars.space['3']}`,
  borderRadius: vars.radii.full,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  fontSize: vars.fontSize.sm,
  boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
  pointerEvents: 'auto',
});

export const reconnectButton = style({
  padding: `2px ${vars.space['2']}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: 'transparent',
  color: 'inherit',
  fontSize: vars.fontSize.sm,
  cursor: 'pointer',
  ':hover': {
    background: vars.color.hoverBackground,
  },
});

export const toolbar = style({
  display: 'flex',
  alignItems: 'center',
  gap: vars.space['2'],
  height: '28px',
  flexShrink: 0,
  padding: `0 ${vars.space['2']}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const inspectButton = style({
  display: 'inline-flex',
  alignItems: 'center',
  gap: vars.space['1'],
  padding: `2px ${vars.space['2']}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: 'transparent',
  color: 'inherit',
  fontSize: vars.fontSize.sm,
  cursor: 'pointer',
  ':hover': { background: vars.color.hoverBackground },
  ':disabled': { opacity: 0.5, cursor: 'not-allowed' },
  selectors: { '&[aria-pressed="true"]': { background: vars.color.hoverBackground, borderColor: 'currentColor' } },
});

export const resultPanel = style({
  position: 'absolute',
  top: 0,
  right: 0,
  bottom: 0,
  width: '320px',
  zIndex: 20,
  overflowY: 'auto',
  display: 'flex',
  flexDirection: 'column',
  gap: vars.space['2'],
  padding: vars.space['3'],
  background: vars.color.cardBackground,
  borderLeft: `1px solid ${vars.color.borderColor}`,
  fontSize: vars.fontSize.sm,
});

export const resultHeader = style({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: vars.space['2'],
});

export const resultScreenshot = style({
  maxWidth: '100%',
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
});

export const resultCode = style({
  margin: 0,
  padding: vars.space['2'],
  maxHeight: '180px',
  overflow: 'auto',
  fontFamily: 'monospace',
  fontSize: '0.75rem',
  whiteSpace: 'pre',
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});

export const cssTable = style({
  width: '100%',
  borderCollapse: 'collapse',
  fontFamily: 'monospace',
  fontSize: '0.75rem',
  selectors: { '& td': { padding: '1px 4px', verticalAlign: 'top', wordBreak: 'break-all' } },
});
