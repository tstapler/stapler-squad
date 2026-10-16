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
