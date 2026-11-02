import { createTheme, type Theme } from "@mui/material/styles";
import type { ThemeParams } from "./telegram";

// themeFrom builds the MUI theme from the user's Telegram colors, so the app looks like
// Telegram in light and dark mode. MUI's defaults fill any color Telegram leaves out.
export function themeFrom(scheme: "light" | "dark", p: ThemeParams): Theme {
  return createTheme({
    palette: {
      mode: scheme,
      ...(p.button_color && {
        primary: { main: p.button_color, ...(p.button_text_color && { contrastText: p.button_text_color }) },
      }),
      ...(p.destructive_text_color && { error: { main: p.destructive_text_color } }),
      background: {
        ...(p.secondary_bg_color && { default: p.secondary_bg_color }),
        ...((p.section_bg_color ?? p.bg_color) && { paper: p.section_bg_color ?? p.bg_color }),
      },
      text: {
        ...(p.text_color && { primary: p.text_color }),
        ...(p.hint_color && { secondary: p.hint_color }),
      },
    },
    shape: { borderRadius: 12 },
    components: {
      MuiButton: { defaultProps: { disableElevation: true }, styleOverrides: { root: { textTransform: "none" } } },
      MuiTab: { styleOverrides: { root: { textTransform: "none", minHeight: 48 } } },
    },
  });
}
