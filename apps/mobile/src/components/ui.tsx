import { Icon, type IconName } from '@/components/Icon';
import { HeaderHeightContext } from 'expo-router/react-navigation';
import React, { useContext, useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Modal,
  Platform,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleProp,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  TextInputProps,
  TextStyle,
  View,
  ViewStyle,
} from 'react-native';
import Animated, {
  FadeInDown,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withTiming,
} from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { errorMessage } from '@/api/client';
import { Backdrop, frosted, Glass } from '@/components/Glass';
import { useMountProgress } from '@/lib/anim';
import { personColor } from '@/lib/colors';
import { formatMoney, formatSigned, initials, minorToMajor } from '@/lib/format';
import { impactFeedback, tapFeedback } from '@/lib/haptics';
import {
  bodyFont,
  displayFont,
  fonts,
  heroGradient,
  MAX_WIDTH,
  Palette,
  radius,
  softShadow,
  spacing,
  TAB_BAR_GAP,
  TAB_BAR_HEIGHT,
  useIsWide,
  useTheme,
} from '@/theme';

export type { IconName };

const DASHBOARD_MAX_WIDTH = 1180;

// ---------- layout ----------
export function Screen({
  children,
  refreshing,
  onRefresh,
  scroll = true,
  contentStyle,
  dashboard,
}: {
  children: React.ReactNode;
  refreshing?: boolean;
  onRefresh?: () => void;
  scroll?: boolean;
  contentStyle?: StyleProp<ViewStyle>;
  /** Overview screens use the full desktop width (pair with `Columns`); others keep a reading column. */
  dashboard?: boolean;
}) {
  const { c } = useTheme();
  const wide = useIsWide();
  // Headers are transparent glass, so the content starts below them and scrolls underneath.
  const headerHeight = useContext(HeaderHeightContext) ?? 0;
  const insets = useSafeAreaInsets();
  // iOS insets the scroll view itself, so the refresh spinner and scroll bar start below the header.
  const insetByScrollView = Platform.OS === 'ios' && scroll;
  const inner = (
    <View
      style={[
        styles.content,
        {
          paddingTop: (insetByScrollView ? 0 : headerHeight) + spacing.md,
          paddingBottom: TAB_BAR_HEIGHT + TAB_BAR_GAP + insets.bottom + spacing.xl,
        },
        dashboard && wide && styles.contentDashboard,
        contentStyle,
      ]}>
      {children}
    </View>
  );
  if (!scroll) {
    return (
      <View style={styles.fill}>
        <Backdrop />
        {inner}
      </View>
    );
  }
  return (
    <View style={styles.fill}>
      <Backdrop />
      <ScrollView
        style={styles.fill}
        contentContainerStyle={styles.scrollContent}
        contentInset={insetByScrollView ? { top: headerHeight } : undefined}
        contentOffset={insetByScrollView ? { x: 0, y: -headerHeight } : undefined}
        scrollIndicatorInsets={insetByScrollView ? { top: headerHeight } : undefined}
        keyboardShouldPersistTaps="handled"
        refreshControl={
          onRefresh ? (
            <RefreshControl
              refreshing={!!refreshing}
              onRefresh={onRefresh}
              tintColor={c.primary}
              colors={[c.accent]}
              progressViewOffset={headerHeight}
            />
          ) : undefined
        }>
        {inner}
      </ScrollView>
    </View>
  );
}

/** Two side-by-side columns on desktop-sized windows; one stacked flow everywhere else. */
export function Columns({ left, right }: { left: React.ReactNode; right: React.ReactNode }) {
  const wide = useIsWide();
  // Same tree in both layouts, so resizing across the breakpoint does not remount the columns.
  return (
    <View style={wide ? styles.columns : styles.columnsStacked}>
      <View style={wide ? styles.column : styles.columnStacked}>{left}</View>
      <View style={wide ? styles.column : styles.columnStacked}>{right}</View>
    </View>
  );
}

export function Row({
  children,
  style,
  gap = spacing.sm,
}: {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  gap?: number;
}) {
  return <View style={[{ flexDirection: 'row', alignItems: 'center', gap }, style]}>{children}</View>;
}

/** Fades + slides children in on mount; `index` staggers list items. */
export function FadeIn({
  children,
  index = 0,
  style,
}: {
  children: React.ReactNode;
  index?: number;
  style?: StyleProp<ViewStyle>;
}) {
  return (
    <Animated.View entering={FadeInDown.duration(320).delay(Math.min(index, 8) * 45)} style={style}>
      {children}
    </Animated.View>
  );
}

// ---------- text ----------
type TxtVariant = 'hero' | 'title' | 'h2' | 'h3' | 'body' | 'muted' | 'small' | 'label' | 'big';

export function Txt({
  children,
  variant = 'body',
  color,
  style,
  numberOfLines,
  weight,
  adjustsFontSizeToFit,
}: {
  children: React.ReactNode;
  variant?: TxtVariant;
  color?: string;
  style?: StyleProp<TextStyle>;
  numberOfLines?: number;
  weight?: TextStyle['fontWeight'];
  adjustsFontSizeToFit?: boolean;
}) {
  const { c } = useTheme();
  const base = textVariants[variant];
  const defaultColor = variant === 'muted' || variant === 'small' || variant === 'label' ? c.textMuted : c.text;
  const family = weight ? { fontFamily: DISPLAY_VARIANTS.has(variant) ? displayFont(weight) : bodyFont(weight) } : null;
  return (
    <Text
      numberOfLines={numberOfLines}
      adjustsFontSizeToFit={adjustsFontSizeToFit}
      minimumFontScale={adjustsFontSizeToFit ? 0.6 : undefined}
      style={[base, { color: color ?? defaultColor }, family, style]}>
      {children}
    </Text>
  );
}

/** Headline variants are set in the display face; a `weight` prop picks a family, never a synthetic bold. */
const DISPLAY_VARIANTS: ReadonlySet<TxtVariant> = new Set(['hero', 'big', 'title', 'h2', 'h3']);

const textVariants: Record<TxtVariant, TextStyle> = {
  hero: { fontFamily: fonts.display, fontSize: 38, lineHeight: 44, letterSpacing: -0.4, fontVariant: ['tabular-nums'] },
  big: { fontFamily: fonts.display, fontSize: 30, lineHeight: 34, letterSpacing: 0, fontVariant: ['tabular-nums'] },
  title: { fontFamily: fonts.display, fontSize: 26, lineHeight: 30, letterSpacing: -0.7 },
  h2: { fontFamily: fonts.display, fontSize: 20, lineHeight: 25, letterSpacing: -0.4 },
  h3: { fontFamily: fonts.display, fontSize: 17, lineHeight: 22, letterSpacing: -0.25 },
  body: { fontFamily: fonts.regular, fontSize: 15, lineHeight: 22 },
  muted: { fontFamily: fonts.regular, fontSize: 14, lineHeight: 20 },
  small: { fontFamily: fonts.regular, fontSize: 13, lineHeight: 18 },
  label: { fontFamily: fonts.semibold, fontSize: 13, lineHeight: 18 },
};

// ---------- surfaces ----------
export function Card({
  children,
  style,
  onPress,
  onLongPress,
  padded = true,
}: {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  onPress?: () => void;
  onLongPress?: () => void;
  padded?: boolean;
}) {
  const { c } = useTheme();
  const s = [styles.card, frosted(c), softShadow(c), padded && styles.cardPad, style];
  if (onPress || onLongPress) {
    return (
      <Pressable
        onPress={() => {
          tapFeedback();
          onPress?.();
        }}
        onLongPress={onLongPress}
        accessibilityRole="button"
        style={({ pressed }) => [s, pressed && styles.pressedScale]}>
        {children}
      </Pressable>
    );
  }
  return <View style={s}>{children}</View>;
}

/** Emerald gradient hero panel (`color` swaps it for a flat fill); text inside should use `onHero` colors. */
export function HeroCard({
  children,
  color,
  style,
  onPress,
}: {
  children: React.ReactNode;
  color?: string;
  style?: StyleProp<ViewStyle>;
  onPress?: () => void;
}) {
  const { c } = useTheme();
  const s = [styles.hero, color ? { backgroundColor: color } : heroGradient(c.hero), softShadow(c, 2), style];
  if (onPress) {
    return (
      <Pressable onPress={onPress} style={({ pressed }) => [s, pressed && styles.pressedScale]}>
        {children}
      </Pressable>
    );
  }
  return <View style={s}>{children}</View>;
}

/** Translucent inset panel for use on hero cards. */
export function GlassPill({ children, style }: { children: React.ReactNode; style?: StyleProp<ViewStyle> }) {
  return <View style={[styles.glass, style]}>{children}</View>;
}

export function SectionHeader({
  title,
  action,
  onAction,
  icon,
}: {
  title: string;
  action?: string;
  onAction?: () => void;
  icon?: IconName;
}) {
  const { c } = useTheme();
  return (
    <Row style={styles.sectionHeader}>
      <Txt variant="h3" style={styles.flex1} numberOfLines={1}>
        {title}
      </Txt>
      {action && onAction ? (
        <Pressable
          onPress={() => {
            tapFeedback();
            onAction();
          }}
          hitSlop={8}
          accessibilityRole="button"
          style={({ pressed }) => [styles.sectionAction, { backgroundColor: c.primarySoft }, pressed && styles.pressed]}>
          {icon ? <Icon name={icon} size={14} color={c.primaryStrong} /> : null}
          <Txt variant="small" color={c.primaryStrong} weight="600">
            {action}
          </Txt>
        </Pressable>
      ) : null}
    </Row>
  );
}

export function ListRow({
  title,
  subtitle,
  left,
  right,
  onPress,
  onLongPress,
  style,
  divider,
}: {
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  left?: React.ReactNode;
  right?: React.ReactNode;
  onPress?: () => void;
  onLongPress?: () => void;
  style?: StyleProp<ViewStyle>;
  divider?: boolean;
}) {
  const { c } = useTheme();
  return (
    <Pressable
      onPress={
        onPress
          ? () => {
              tapFeedback();
              onPress();
            }
          : undefined
      }
      onLongPress={onLongPress}
      disabled={!onPress && !onLongPress}
      style={({ pressed }) => [
        styles.listRow,
        divider && { borderTopWidth: StyleSheet.hairlineWidth, borderColor: c.border },
        pressed && { backgroundColor: c.cardAlt },
        style,
      ]}>
      {left}
      <View style={styles.flex1}>
        {typeof title === 'string' ? (
          <Txt variant="body" weight="600" numberOfLines={1}>
            {title}
          </Txt>
        ) : (
          title
        )}
        {subtitle ? (
          typeof subtitle === 'string' ? (
            <Txt variant="small" numberOfLines={2}>
              {subtitle}
            </Txt>
          ) : (
            subtitle
          )
        ) : null}
      </View>
      {right}
    </Pressable>
  );
}

/** Rounded colored square with an icon (categories, actions, receipts). */
export function IconTile({
  icon,
  color,
  size = 40,
  solid,
}: {
  icon?: IconName;
  color: string;
  size?: number;
  /** Filled tile with white glyph instead of a tinted one. */
  solid?: boolean;
}) {
  const filled = !!solid;
  return (
    <View
      style={[
        styles.tile,
        { width: size, height: size, borderRadius: size * 0.3, backgroundColor: filled ? color : `${color}22` },
      ]}>
      {icon ? (
        <Icon name={icon} size={size * 0.52} color={filled ? '#FFFFFF' : color} weight={filled ? 'fill' : 'duotone'} />
      ) : null}
    </View>
  );
}

/** Small status pill. */
export function Badge({
  label,
  tone = 'muted',
  icon,
}: {
  label: string;
  tone?: 'muted' | 'warn' | 'ok' | 'bad' | 'brand';
  icon?: IconName;
}) {
  const { c } = useTheme();
  const t = {
    muted: { bg: c.cardAlt, fg: c.textMuted },
    warn: { bg: c.warningSoft, fg: c.warning },
    ok: { bg: c.positiveSoft, fg: c.primaryStrong },
    bad: { bg: c.dangerSoft, fg: c.danger },
    brand: { bg: c.primarySoft, fg: c.primaryStrong },
  }[tone];
  return (
    <Row gap={4} style={[styles.badge, { backgroundColor: t.bg }]}>
      {icon ? <Icon name={icon} size={12} color={t.fg} /> : null}
      <Txt variant="small" weight="600" color={t.fg} numberOfLines={1}>
        {label}
      </Txt>
    </Row>
  );
}

// ---------- buttons ----------
type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

export function Button({
  title,
  onPress,
  variant = 'primary',
  loading,
  disabled,
  icon,
  small,
  style,
  textColor,
}: {
  title: string;
  onPress: () => void;
  variant?: ButtonVariant;
  loading?: boolean;
  disabled?: boolean;
  icon?: IconName;
  small?: boolean;
  style?: StyleProp<ViewStyle>;
  /** Overrides the label color when `style` repaints the button (e.g. on a hero panel). */
  textColor?: string;
}) {
  const { c } = useTheme();
  const base = buttonPalette(c, variant);
  const palette = textColor ? { ...base, fg: textColor } : base;
  const isDisabled = disabled || loading;
  return (
    <Pressable
      onPress={() => {
        if (variant === 'primary') impactFeedback();
        else tapFeedback();
        onPress();
      }}
      disabled={isDisabled}
      accessibilityRole="button"
      accessibilityState={{ disabled: !!isDisabled, busy: !!loading }}
      style={({ pressed }) => [
        styles.button,
        small && styles.buttonSmall,
        { backgroundColor: palette.bg, borderColor: palette.border },
        variant === 'primary' && !isDisabled && { boxShadow: `0px 8px 22px ${c.accentGlow}` },
        pressed && styles.pressedScale,
        isDisabled && styles.disabled,
        style,
      ]}>
      {loading ? (
        <ActivityIndicator color={palette.fg} size="small" />
      ) : (
        <Row gap={6} style={styles.center}>
          {icon ? <Icon name={icon} size={small ? 16 : 18} color={palette.fg} /> : null}
          <Text numberOfLines={1} style={[styles.buttonText, small && styles.buttonTextSmall, { color: palette.fg }]}>
            {title}
          </Text>
        </Row>
      )}
    </Pressable>
  );
}

function buttonPalette(c: Palette, v: ButtonVariant): { bg: string; fg: string; border: string } {
  switch (v) {
    case 'primary':
      return { bg: c.accent, fg: c.primaryText, border: 'transparent' };
    case 'secondary':
      return { bg: c.primarySoft, fg: c.primaryStrong, border: c.primarySoft };
    case 'danger':
      return { bg: c.dangerSoft, fg: c.danger, border: c.dangerSoft };
    case 'ghost':
    default:
      return { bg: 'transparent', fg: c.primaryStrong, border: c.outline };
  }
}

export function IconButton({
  icon,
  onPress,
  color,
  size = 22,
  label,
  filled,
}: {
  icon: IconName;
  onPress: () => void;
  color?: string;
  size?: number;
  label?: string;
  filled?: boolean;
}) {
  const { c } = useTheme();
  return (
    <Pressable
      onPress={() => {
        tapFeedback();
        onPress();
      }}
      hitSlop={6}
      accessibilityLabel={label}
      accessibilityRole="button"
      style={({ pressed }) => [
        styles.iconButton,
        filled && { backgroundColor: c.cardAlt },
        pressed && styles.pressed,
      ]}>
      <Icon name={icon} size={size} color={color ?? c.text} />
    </Pressable>
  );
}

// ---------- inputs ----------
export function Field({
  label,
  error,
  style,
  ...props
}: TextInputProps & { label?: string; error?: string | null }) {
  const { c } = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <View style={[styles.field, style as StyleProp<ViewStyle>]}>
      {label ? <Txt variant="label">{label}</Txt> : null}
      <TextInput
        placeholderTextColor={c.textMuted}
        {...props}
        onFocus={(e) => {
          setFocused(true);
          props.onFocus?.(e);
        }}
        onBlur={(e) => {
          setFocused(false);
          props.onBlur?.(e);
        }}
        style={[
          styles.input,
          {
            color: c.text,
            backgroundColor: focused ? c.card : c.cardAlt,
            borderColor: error ? c.danger : focused ? c.primary : c.border,
          },
          props.multiline && styles.inputMultiline,
        ]}
      />
      {error ? (
        <Txt variant="small" color={c.danger}>
          {error}
        </Txt>
      ) : null}
    </View>
  );
}

export function ToggleRow({
  label,
  value,
  onChange,
  hint,
}: {
  label: string;
  value: boolean;
  onChange: (v: boolean) => void;
  hint?: string;
}) {
  const { c } = useTheme();
  return (
    <Row style={styles.toggleRow}>
      <View style={styles.flex1}>
        <Txt weight="600">{label}</Txt>
        {hint ? <Txt variant="small">{hint}</Txt> : null}
      </View>
      <Switch
        value={value}
        onValueChange={(v) => {
          tapFeedback();
          onChange(v);
        }}
        trackColor={{ true: c.accent, false: c.outline }}
        thumbColor={value ? c.primaryText : c.card}
      />
    </Row>
  );
}

export function Segmented<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string; icon?: IconName }[];
  value: T;
  onChange: (v: T) => void;
}) {
  const { c } = useTheme();
  return (
    <Row gap={4} style={[styles.segmented, { backgroundColor: c.cardAlt }]}>
      {options.map((o) => {
        const active = o.value === value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="tab"
            accessibilityState={{ selected: active }}
            onPress={() => {
              tapFeedback();
              onChange(o.value);
            }}
            style={[styles.segment, active && [{ backgroundColor: c.card }, softShadow(c)]]}>
            <Row gap={6} style={styles.center}>
              {o.icon ? <Icon name={o.icon} size={15} color={active ? c.primary : c.textMuted} /> : null}
              <Txt variant="muted" weight={active ? '700' : '500'} color={active ? c.text : c.textMuted} numberOfLines={1}>
                {o.label}
              </Txt>
            </Row>
          </Pressable>
        );
      })}
    </Row>
  );
}

export function Chip({
  label,
  active,
  onPress,
  icon,
}: {
  label: string;
  active?: boolean;
  onPress?: () => void;
  icon?: IconName;
}) {
  const { c } = useTheme();
  return (
    <Pressable
      onPress={
        onPress
          ? () => {
              tapFeedback();
              onPress();
            }
          : undefined
      }
      disabled={!onPress}
      style={({ pressed }) => [
        styles.chip,
        { backgroundColor: active ? c.accent : c.cardAlt, borderColor: active ? c.accent : c.border },
        pressed && styles.pressed,
      ]}>
      <Row gap={4}>
        {icon ? <Icon name={icon} size={14} color={active ? c.primaryText : c.text} /> : null}
        <Txt variant="muted" weight="600" color={active ? c.primaryText : c.text}>
          {label}
        </Txt>
      </Row>
    </Pressable>
  );
}

export function Stepper({
  value,
  onChange,
  min = 0,
  max = 99,
}: {
  value: number;
  onChange: (v: number) => void;
  min?: number;
  max?: number;
}) {
  const { c } = useTheme();
  return (
    <Row gap={2} style={[styles.stepper, { borderColor: c.border, backgroundColor: c.card }]}>
      <IconButton icon="remove" size={18} onPress={() => onChange(Math.max(min, value - 1))} label="Decrease" />
      <Txt weight="700" style={styles.stepperValue}>
        {value}
      </Txt>
      <IconButton icon="add" size={18} onPress={() => onChange(Math.min(max, value + 1))} label="Increase" />
    </Row>
  );
}

// ---------- avatars ----------
export function Avatar({
  name,
  id,
  size = 38,
  ring,
}: {
  name?: string;
  id?: string;
  size?: number;
  /** Border color, e.g. the surface color for stacked avatars. */
  ring?: string;
}) {
  const color = personColor(id || name || '?');
  return (
    <View
      accessibilityLabel={name}
      style={[
        styles.avatar,
        { width: size, height: size, borderRadius: size / 2, backgroundColor: color },
        ring ? { borderWidth: 2, borderColor: ring } : null,
      ]}>
      <Text style={{ color: '#FFFFFF', fontFamily: fonts.semibold, fontSize: size * 0.38 }}>{initials(name)}</Text>
    </View>
  );
}

export function AvatarStack({
  people,
  size = 24,
  max = 4,
  ring,
}: {
  people: { id: string; name: string }[];
  size?: number;
  max?: number;
  ring?: string;
}) {
  const { c } = useTheme();
  const shown = people.slice(0, max);
  const rest = people.length - shown.length;
  return (
    <Row gap={0}>
      {shown.map((p, i) => (
        <View key={p.id} style={{ marginLeft: i === 0 ? 0 : -size / 4, zIndex: max - i }}>
          <Avatar name={p.name} id={p.id} size={size} ring={ring ?? c.card} />
        </View>
      ))}
      {rest > 0 ? (
        <View
          style={[
            styles.avatar,
            { width: size, height: size, borderRadius: size / 2, marginLeft: -size / 4, backgroundColor: c.card, borderWidth: 2, borderColor: ring ?? c.card },
          ]}>
          <Text style={{ color: c.text, fontFamily: fonts.semibold, fontSize: size * 0.36 }}>+{rest}</Text>
        </View>
      ) : null}
    </Row>
  );
}

// ---------- states ----------
export function Banner({
  kind = 'warning',
  title,
  message,
  icon,
}: {
  kind?: 'warning' | 'error' | 'info' | 'success';
  title?: string;
  message: string;
  icon?: IconName;
}) {
  const { c } = useTheme();
  const map = {
    warning: { bg: c.warningSoft, fg: c.warning, icon: 'warning' as IconName },
    error: { bg: c.dangerSoft, fg: c.danger, icon: 'alert-circle' as IconName },
    info: { bg: c.primarySoft, fg: c.primaryStrong, icon: 'information-circle' as IconName },
    success: { bg: c.positiveSoft, fg: c.primaryStrong, icon: 'checkmark-circle' as IconName },
  }[kind];
  return (
    <Row style={[styles.banner, { backgroundColor: map.bg }]} gap={10}>
      <Icon name={icon ?? map.icon} size={20} color={map.fg} />
      <View style={styles.flex1}>
        {title ? (
          <Txt weight="700" color={map.fg}>
            {title}
          </Txt>
        ) : null}
        <Txt variant="muted" color={kind === 'info' ? c.text : map.fg}>
          {message}
        </Txt>
      </View>
    </Row>
  );
}

/** Pulsing placeholder block. */
export function Skeleton({
  width = '100%',
  height = 14,
  r = 8,
  style,
}: {
  width?: ViewStyle['width'];
  height?: number;
  r?: number;
  style?: StyleProp<ViewStyle>;
}) {
  const { c } = useTheme();
  const o = useSharedValue(0.55);
  useEffect(() => {
    o.value = withRepeat(withTiming(1, { duration: 750 }), -1, true);
  }, [o]);
  const anim = useAnimatedStyle(() => ({ opacity: o.value }));
  return <Animated.View style={[{ width, height, borderRadius: r, backgroundColor: c.skeleton }, anim, style]} />;
}

/** Skeleton of a list of rows inside cards (used instead of spinners on main lists). */
export function SkeletonList({ rows = 3, card = true }: { rows?: number; card?: boolean }) {
  const items = Array.from({ length: rows }, (_, i) => (
    <Row key={i} gap={spacing.md} style={styles.skelRow}>
      <Skeleton width={42} height={42} r={14} />
      <View style={[styles.flex1, { gap: 8 }]}>
        <Skeleton width={`${70 - ((i * 13) % 30)}%`} height={14} />
        <Skeleton width={`${40 + ((i * 17) % 25)}%`} height={11} />
      </View>
      <Skeleton width={64} height={16} />
    </Row>
  ));
  if (!card) return <View accessibilityLabel="Loading">{items}</View>;
  return (
    <Card>
      <View accessibilityLabel="Loading">{items}</View>
    </Card>
  );
}

export function LoadingView({ label }: { label?: string }) {
  const { c } = useTheme();
  return (
    <View style={styles.stateBox}>
      <ActivityIndicator color={c.primary} size="large" />
      {label ? <Txt variant="muted">{label}</Txt> : null}
    </View>
  );
}

export function ErrorView({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { c } = useTheme();
  return (
    <View style={styles.stateBox}>
      <View style={[styles.emptyIcon, { backgroundColor: c.dangerSoft }]}>
        <Icon name="cloud-offline" size={28} color={c.danger} />
      </View>
      <Txt variant="muted" style={styles.textCenter}>
        {errorMessage(error)}
      </Txt>
      {onRetry ? <Button title="Try again" icon="refresh" variant="secondary" small onPress={onRetry} /> : null}
    </View>
  );
}

export function EmptyView({
  icon = 'file-tray-outline',
  title,
  message,
  action,
}: {
  icon?: IconName;
  title: string;
  message?: string;
  action?: React.ReactNode;
}) {
  const { c } = useTheme();
  return (
    <View style={styles.stateBox}>
      <View style={[styles.emptyArt, { backgroundColor: c.primarySoft }]}>
        <View style={[styles.emptyArtInner, { backgroundColor: c.card }]}>
          <Icon name={icon} size={34} color={c.primary} weight="duotone" />
        </View>
      </View>
      <Txt variant="h2" style={styles.textCenter}>
        {title}
      </Txt>
      {message ? (
        <Txt variant="muted" style={[styles.textCenter, styles.emptyMsg]}>
          {message}
        </Txt>
      ) : null}
      {action}
    </View>
  );
}

/** Renders loading / error states for a query and children when data is ready. */
export function QueryState<T>({
  query,
  children,
  skeleton,
}: {
  query: { data: T | undefined; isLoading: boolean; error: unknown; refetch: () => unknown };
  children: (data: T) => React.ReactNode;
  skeleton?: React.ReactNode;
}) {
  if (query.data !== undefined) return <>{children(query.data)}</>;
  if (query.error) return <ErrorView error={query.error} onRetry={() => void query.refetch()} />;
  return <>{skeleton ?? <SkeletonList />}</>;
}

export function Money({
  cents,
  currency,
  signed,
  colored,
  variant = 'body',
  style,
  color: colorProp,
  animate,
  numberOfLines,
  fitChars,
}: {
  cents: number;
  currency: string;
  signed?: boolean;
  colored?: boolean;
  color?: string;
  variant?: TxtVariant;
  style?: StyleProp<TextStyle>;
  /** Count up from 0 on mount. */
  animate?: boolean;
  numberOfLines?: number;
  /** Shrink the font when the text is longer than this many characters (tight layouts). */
  fitChars?: number;
}) {
  const { c } = useTheme();
  const p = useMountProgress(cents, 800, !!animate);
  const shown = animate ? Math.round(cents * p) : cents;
  const color = colored ? (cents > 0 ? c.positive : cents < 0 ? c.negative : c.textMuted) : colorProp;
  // Headline amounts drop the fraction when it is zero or the amount is 10 000+, so they fit on one line.
  const major = minorToMajor(cents, currency || 'USD');
  const whole = (variant === 'hero' || variant === 'big') && (Math.abs(major) >= 10000 || Number.isInteger(major));
  const text = signed ? formatSigned(shown, currency || 'USD', whole) : formatMoney(shown, currency || 'USD', whole);
  // Size by the final amount so the count-up animation doesn't change the font size.
  const finalLen = (signed ? formatSigned(cents, currency || 'USD', whole) : formatMoney(cents, currency || 'USD', whole)).length;
  const base = textVariants[variant]?.fontSize;
  const fit =
    fitChars && base && finalLen > fitChars
      ? { fontSize: Math.round((base * fitChars) / finalLen), lineHeight: Math.round((base * fitChars * 1.2) / finalLen) }
      : null;
  return (
    <Txt
      variant={variant}
      color={color}
      weight={variant === 'body' || variant === 'muted' || variant === 'small' ? '700' : undefined}
      style={[{ fontVariant: ['tabular-nums'] }, fit, style]}
      numberOfLines={numberOfLines ?? 1}>
      {text}
    </Txt>
  );
}

// ---------- pickers ----------
export interface PickerOption {
  value: string;
  label: string;
  hint?: string;
}

export function SelectModal({
  visible,
  title,
  options,
  selected,
  onSelect,
  onClose,
  searchable,
}: {
  visible: boolean;
  title: string;
  options: PickerOption[];
  selected?: string;
  onSelect: (value: string) => void;
  onClose: () => void;
  searchable?: boolean;
}) {
  const { c } = useTheme();
  const [q, setQ] = useState('');
  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return options;
    return options.filter((o) => o.label.toLowerCase().includes(s) || (o.hint ?? '').toLowerCase().includes(s));
  }, [options, q]);
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={styles.modalBackdrop} onPress={onClose}>
        <Pressable onPress={() => undefined} style={styles.modalSheetWrap}>
          <Glass style={styles.modalSheet}>
          <Row style={styles.modalHeader}>
            <Txt variant="h2" style={styles.flex1}>
              {title}
            </Txt>
            <IconButton icon="close" onPress={onClose} label="Close" filled />
          </Row>
          {searchable ? <Field placeholder="Search" value={q} onChangeText={setQ} autoCapitalize="none" /> : null}
          <FlatList
            data={filtered}
            keyExtractor={(o) => o.value}
            style={styles.modalList}
            keyboardShouldPersistTaps="handled"
            ListEmptyComponent={<Txt variant="muted">Nothing found</Txt>}
            renderItem={({ item }) => (
              <ListRow
                title={item.label}
                subtitle={item.hint}
                style={styles.modalRow}
                onPress={() => {
                  onSelect(item.value);
                  setQ('');
                  onClose();
                }}
                right={
                  item.value === selected ? <Icon name="checkmark-circle" size={22} color={c.primary} /> : undefined
                }
              />
            )}
          />
          </Glass>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

/** A field-looking button that opens a SelectModal. */
export function SelectField({
  label,
  value,
  placeholder,
  options,
  onChange,
  searchable,
}: {
  label?: string;
  value: string;
  placeholder?: string;
  options: PickerOption[];
  onChange: (v: string) => void;
  searchable?: boolean;
}) {
  const { c } = useTheme();
  const [open, setOpen] = useState(false);
  const current = options.find((o) => o.value === value);
  return (
    <View style={styles.field}>
      {label ? <Txt variant="label">{label}</Txt> : null}
      <Pressable
        onPress={() => {
          tapFeedback();
          setOpen(true);
        }}
        accessibilityRole="button"
        style={[styles.input, styles.selectInput, { backgroundColor: c.cardAlt, borderColor: c.border }]}>
        <Txt color={current ? c.text : c.textMuted} style={styles.flex1} numberOfLines={1}>
          {current ? current.label : value || placeholder || 'Select…'}
        </Txt>
        <Icon name="chevron-down" size={18} color={c.textMuted} />
      </Pressable>
      <SelectModal
        visible={open}
        title={label ?? 'Select'}
        options={options}
        selected={value}
        onSelect={onChange}
        onClose={() => setOpen(false)}
        searchable={searchable}
      />
    </View>
  );
}

export function Divider() {
  const { c } = useTheme();
  return <View style={{ height: StyleSheet.hairlineWidth, backgroundColor: c.border, marginVertical: spacing.sm }} />;
}

export const styles = StyleSheet.create({
  fill: { flex: 1 },
  flex1: { flex: 1 },
  center: { justifyContent: 'center' },
  textCenter: { textAlign: 'center' },
  scrollContent: { flexGrow: 1 },
  content: {
    width: '100%',
    maxWidth: MAX_WIDTH,
    alignSelf: 'center',
    padding: spacing.lg,
    gap: spacing.md,
  },
  contentDashboard: { maxWidth: DASHBOARD_MAX_WIDTH, paddingHorizontal: spacing.xxl },
  columns: { flexDirection: 'row', alignItems: 'flex-start', gap: spacing.xl },
  column: { flex: 1, minWidth: 0, gap: spacing.md },
  columnsStacked: { gap: spacing.md },
  columnStacked: { gap: spacing.md },
  card: { borderRadius: radius.lg, borderWidth: 1, overflow: 'hidden' },
  cardPad: { padding: spacing.lg + 2, gap: spacing.sm },
  hero: {
    borderRadius: radius.xl,
    padding: spacing.xl,
    gap: spacing.sm,
    overflow: 'hidden',
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.20)',
  },
  glass: {
    flex: 1,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.md + 2,
    borderRadius: radius.md,
    gap: 2,
    backgroundColor: 'rgba(255,255,255,0.13)',
  },
  pressed: { opacity: 0.7 },
  pressedScale: { opacity: 0.92, transform: [{ scale: 0.985 }] },
  disabled: { opacity: 0.45 },
  sectionHeader: { marginTop: spacing.md },
  sectionAction: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    paddingHorizontal: 12,
    minHeight: 32,
    borderRadius: radius.pill,
  },
  listRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingVertical: spacing.md,
    minHeight: 56,
  },
  tile: { alignItems: 'center', justifyContent: 'center', overflow: 'hidden' },
  badge: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill, alignSelf: 'flex-start' },
  button: {
    minHeight: 52,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.xl,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1.5,
    overflow: 'hidden',
  },
  buttonSmall: { minHeight: 44, paddingHorizontal: spacing.lg },
  buttonText: { fontFamily: fonts.semibold, fontSize: 16 },
  buttonTextSmall: { fontSize: 14 },
  iconButton: { minWidth: 44, minHeight: 44, alignItems: 'center', justifyContent: 'center', borderRadius: radius.pill },
  field: { gap: 6 },
  input: {
    minHeight: 52,
    borderWidth: 1.5,
    borderRadius: radius.md,
    paddingHorizontal: spacing.lg,
    fontFamily: fonts.regular,
    fontSize: 16,
  },
  inputMultiline: { minHeight: 110, paddingTop: spacing.md, textAlignVertical: 'top' },
  selectInput: { flexDirection: 'row', alignItems: 'center' },
  toggleRow: { paddingVertical: spacing.sm },
  segmented: { borderRadius: radius.pill, padding: 4 },
  segment: { flex: 1, alignItems: 'center', justifyContent: 'center', minHeight: 42, borderRadius: radius.pill },
  chip: { paddingHorizontal: 14, minHeight: 36, justifyContent: 'center', borderRadius: radius.pill, borderWidth: 1 },
  stepper: { borderWidth: 1, borderRadius: radius.pill, paddingHorizontal: 2 },
  stepperValue: { minWidth: 22, textAlign: 'center' },
  avatar: { alignItems: 'center', justifyContent: 'center', overflow: 'hidden' },
  banner: { padding: spacing.md, borderRadius: radius.md, alignItems: 'flex-start' },
  stateBox: { alignItems: 'center', justifyContent: 'center', padding: spacing.xl, gap: spacing.md },
  emptyIcon: { width: 56, height: 56, borderRadius: 28, alignItems: 'center', justifyContent: 'center' },
  emptyArt: { width: 104, height: 104, borderRadius: 52, alignItems: 'center', justifyContent: 'center' },
  emptyArtInner: { width: 72, height: 72, borderRadius: 36, alignItems: 'center', justifyContent: 'center' },
  emptyMsg: { maxWidth: 340 },
  skelRow: { paddingVertical: spacing.sm },
  modalBackdrop: {
    flex: 1,
    backgroundColor: 'rgba(13,20,16,0.55)',
    justifyContent: 'center',
    alignItems: 'center',
    padding: spacing.lg,
  },
  modalSheetWrap: { width: '100%', maxWidth: 480, maxHeight: '80%' },
  modalSheet: {
    flexShrink: 1,
    borderRadius: radius.xl,
    padding: spacing.lg,
    gap: spacing.sm,
  },
  modalHeader: { marginBottom: spacing.xs },
  modalList: { flexGrow: 0, flexShrink: 1 },
  modalRow: { paddingHorizontal: spacing.sm },
});
