// @vitest-environment node
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import type { IncomingMessage, ServerResponse } from 'node:http'
import type { ViteDevServer } from 'vite'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import themeDev from './theme-dev-plugin.ts'

const index = '<!doctype html>\n<html lang="en">\n  <head>\n  </head>\n  <body></body>\n</html>\n'

let themes: string

beforeEach(() => {
  themes = fs.mkdtempSync(path.join(os.tmpdir(), 'themes-'))
  fs.mkdirSync(path.join(themes, 'demo'))
  fs.writeFileSync(path.join(themes, 'demo', 'theme.json'), '{"smugbox": 1, "name": "</script>", "dark": true, "gallery": "masonry"}')
  fs.writeFileSync(path.join(themes, 'demo', 'theme.css'), 'body{}')
  fs.writeFileSync(path.join(themes, 'demo', 'README.md'), 'x')
  fs.writeFileSync(path.join(themes, 'secret.css'), 'secret')
  vi.stubEnv('SITE_THEME', 'demo')
  vi.stubEnv('SITE_TITLE', '<b>Site</b>')
})
afterEach(() => {
  vi.unstubAllEnvs()
  fs.rmSync(themes, { recursive: true })
})

function transform(html: string): string {
  return (themeDev(themes).transformIndexHtml as (html: string) => string)(html)
}

type Middleware = (req: IncomingMessage, res: ServerResponse) => void

function serve(url: string) {
  let mounted: { prefix: string; fn: Middleware } | undefined
  const server = { middlewares: { use: (prefix: string, fn: Middleware) => (mounted = { prefix, fn }) } }
  ;(themeDev(themes).configureServer as (s: ViteDevServer) => void)(server as unknown as ViteDevServer)
  expect(mounted?.prefix).toBe('/theme/dev')
  const res = { statusCode: 200, headers: {} as Record<string, string>, body: '' }
  mounted!.fn({ url: url.slice('/theme/dev'.length) } as IncomingMessage, {
    setHeader: (k: string, v: string) => (res.headers[k] = v),
    end: (b?: Buffer) => (res.body = b ? b.toString() : ''),
    set statusCode(c: number) {
      res.statusCode = c
    },
  } as unknown as ServerResponse)
  return res
}

describe('theme dev plugin', () => {
  it('only runs on the dev server', () => {
    expect(themeDev(themes).apply).toBe('serve')
  })

  it('injects the title, stylesheet, dark class and site JSON once each', () => {
    const html = transform(index)
    for (const want of [
      '<title>&lt;b&gt;Site&lt;/b&gt;</title>',
      '<link rel="stylesheet" href="/theme/dev/theme.css">',
      '<html class="dark" lang="en">',
      'id="smugbox-site"',
    ]) {
      expect(html.split(want).length - 1, want).toBe(1)
    }
    expect(html.split('</script>').length - 1).toBe(1)
    const json = html.split('id="smugbox-site">')[1].split('</script>')[0]
    expect(JSON.parse(json)).toEqual({
      title: '<b>Site</b>',
      theme: { smugbox: 1, name: '</script>', dark: true, gallery: 'masonry' },
    })
  })

  it('adds no class for a light theme', () => {
    fs.writeFileSync(path.join(themes, 'demo', 'theme.json'), '{"smugbox": 1, "name": "L"}')
    expect(transform(index)).toContain('<html lang="en">')
  })

  it('serves the theme folder at /theme/dev/', () => {
    const res = serve('/theme/dev/theme.css')
    expect(res).toMatchObject({ statusCode: 200, body: 'body{}' })
    expect(res.headers['Content-Type']).toBe('text/css; charset=utf-8')
    expect(serve('/theme/dev/missing.css').statusCode).toBe(404)
    expect(serve('/theme/dev/README.md').statusCode).toBe(404)
    expect(serve('/theme/dev/../secret.css').statusCode).toBe(404)
    expect(serve('/theme/dev/%2e%2e/secret.css').statusCode).toBe(404)
  })
})
