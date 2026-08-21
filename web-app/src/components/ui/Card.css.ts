import { style } from '@vanilla-extract/css';
import { recipe, type RecipeVariants } from '@vanilla-extract/recipes';
import { vars } from '@/styles/theme.css';

export const card = recipe({
  base: {
    background: vars.color.cardBackground,
    borderRadius: vars.radii.lg,
    border: `1px solid ${vars.color.borderColor}`,
    overflow: 'hidden',
  },
  variants: {
    variant: {
      default: {},
      elevated: {
        boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
        borderColor: vars.color.borderSubtle,
      },
      bordered: {
        border: `2px solid ${vars.color.borderColor}`,
      },
      interactive: {
        cursor: 'pointer',
        selectors: {
          '&:hover': {
            borderColor: vars.color.primary,
            background: vars.color.hoverBackground,
          },
        },
      },
    },
    padding: {
      none: { padding: '0' },
      sm: { padding: vars.space['3'] },
      md: { padding: vars.space['4'] },
      lg: { padding: vars.space['6'] },
    },
  },
  defaultVariants: {
    variant: 'default',
    padding: 'md',
  },
});

export const cardHeader = style({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  paddingBottom: vars.space['3'],
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  marginBottom: vars.space['3'],
});

export const cardTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: '600',
  color: vars.color.textPrimary,
});

export const cardDescription = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  marginTop: vars.space['1'],
});

export const cardFooter = style({
  display: 'flex',
  alignItems: 'center',
  gap: vars.space['2'],
  paddingTop: vars.space['3'],
  borderTop: `1px solid ${vars.color.borderSubtle}`,
  marginTop: vars.space['3'],
});

export type CardVariants = RecipeVariants<typeof card>;
