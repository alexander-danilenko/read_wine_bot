import { createContext, useContext } from "react";
import en from "./locales/en.json";
import uk from "./locales/uk.json";

export type Lang = "en" | "uk";
export type TextKey = keyof typeof en;

const texts: Record<Lang, Record<TextKey, string>> = { en, uk };

// translator returns t(key, vars) for lang: the text with {name} placeholders filled in.
export function translator(lang: Lang) {
  return (key: TextKey, vars: Record<string, string | number> = {}): string =>
    texts[lang][key].replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? ""));
}

export type T = ReturnType<typeof translator>;

export const TextContext = createContext<T>(translator("uk"));

export function useT(): T {
  return useContext(TextContext);
}
