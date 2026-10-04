// @vitest-environment node
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// Read from disk: Vitest runs with css: false, which empties CSS imports.
const src = path.join(import.meta.dirname, 'src')
const css = fs.readFileSync(path.join(src, 'index.css'), 'utf8')

// Selectors of rules at the top level of a stylesheet, i.e. outside any
// @layer (or other) block. Comments are stripped; strings with braces are not
// handled, index.css has none.
function topLevelSelectors(source: string): string[] {
  const css = source.replace(/\/\*[\s\S]*?\*\//g, '')
  const out: string[] = []
  let depth = 0
  let start = 0
  for (let i = 0; i < css.length; i++) {
    const c = css[i]
    if (c === '{') {
      if (depth === 0) out.push(css.slice(start, i).trim())
      depth++
    } else if (c === '}') {
      depth--
      if (depth === 0) start = i + 1
    } else if (c === ';' && depth === 0) {
      start = i + 1
    }
  }
  return out
}

const tokenBlock = (s: string) => /(^|,)\s*(:root|\.dark)\b/.test(s)

// Theme CSS is unlayered and wins over core only because all core CSS is
// layered and nothing in core is !important (an important declaration in a
// layer beats the theme's normal one). `npx shadcn add` can break both: it
// may append an unlayered :root/.dark block, and its components use
// Tailwind's `!` suffix.
describe('core CSS', () => {
  it('uses no !important or Tailwind ! utilities', () => {
    const files = fs.readdirSync(src, { recursive: true, encoding: 'utf8' }).filter((f) => /\.(tsx?|css)$/.test(f))
    expect(files.length).toBeGreaterThan(10)
    const hits = files.flatMap((f) =>
      fs
        .readFileSync(path.join(src, f), 'utf8')
        .split('\n')
        .filter((line) => /!important|[\w)\]]!(?=[\s"'`]|$)/.test(line))
        .map((line) => `${f}: ${line.trim()}`),
    )
    expect(hits).toEqual([])
  })


  it('has no :root or .dark block outside @layer', () => {
    expect(css).toContain(':root')
    expect(topLevelSelectors(css).filter(tokenBlock)).toEqual([])
  })

  it('the check catches an unlayered block', () => {
    const sample = '@layer base { :root { --a: 1 } }\n@custom-variant dark (&:is(.dark *));\n.dark, .x { --a: 2 }'
    expect(topLevelSelectors(sample).filter(tokenBlock)).toEqual(['.dark, .x'])
  })
})
