// Compiles design/tokens.json (the Concession design system's tokens) into
// src/styles/tokens.css. Run after changing the tokens: `npm run tokens`.
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('..', import.meta.url))
const tokens = JSON.parse(readFileSync(`${root}../design/tokens.json`, 'utf8'))
const themes = tokens.color.themes.map((t) => t.id)
const [light, dark] = themes

const css = (v) => (typeof v === 'string' ? v.replace(/^\{(.+)\}$/, 'var(--$1)') : v)
const pick = (value, theme) => (typeof value === 'string' ? (theme === light ? value : undefined) : value[theme])

function themeVars(theme, indent) {
  const out = []
  for (const family of ['color', 'shadow']) {
    for (const t of tokens[family].tokens) {
      const v = pick(t.value, theme)
      if (v !== undefined) out.push(`${indent}--${t.name}: ${css(v)};`)
    }
  }
  return out.join('\n')
}

const out = `/* Generated from design/tokens.json by scripts/tokens-css.mjs. Do not edit by hand. */
:root,
[data-theme="${light}"] {
  color-scheme: light;
${themeVars(light, '  ')}
}

[data-theme="${dark}"] {
  color-scheme: dark;
${themeVars(dark, '  ')}
}

@media (prefers-color-scheme: dark) {
  :root:not([data-theme="${light}"]) {
    color-scheme: dark;
${themeVars(dark, '    ')}
  }
}
`
writeFileSync(`${root}src/styles/tokens.css`, out)
console.log('wrote src/styles/tokens.css')
