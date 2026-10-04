import fs from 'node:fs'
import path from 'node:path'
import type { Plugin } from 'vite'

// Same table as theme.ContentTypes in backend/internal/theme.
const contentTypes: Record<string, string> = {
  '.css': 'text/css; charset=utf-8',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.webp': 'image/webp',
  '.avif': 'image/avif',
}

const escapeHTML = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

/**
 * The dev server's stand-in for the backend's theme injection
 * (backend/internal/web): puts the title, the theme stylesheet, the dark
 * class and the #smugbox-site JSON into index.html, and serves the theme
 * folder `themes/$SITE_THEME` (default noir) at /theme/dev/. It does not
 * validate the theme; the Go loader is the only validator. Not used by
 * `vite build`, whose index.html the backend rewrites at startup.
 */
export default function themeDev(themesDir = path.resolve(import.meta.dirname, '../themes')): Plugin {
  const dir = path.join(themesDir, process.env.SITE_THEME || 'noir')
  const title = process.env.SITE_TITLE || 'Smugbox'
  return {
    name: 'smugbox-theme-dev',
    apply: 'serve',
    transformIndexHtml(html) {
      // Read on every page load, so theme.json edits show up on reload.
      const manifest = JSON.parse(fs.readFileSync(path.join(dir, 'theme.json'), 'utf8'))
      const site = JSON.stringify({ title, theme: manifest }).replace(/</g, '\\u003c')
      const head =
        `  <title>${escapeHTML(title)}</title>\n` +
        `    <link rel="stylesheet" href="/theme/dev/theme.css">\n` +
        `    <script type="application/json" id="smugbox-site">${site}</script>\n  `
      html = html.replace('</head>', head + '</head>')
      return manifest.dark ? html.replace('<html', '<html class="dark"') : html
    },
    configureServer(server) {
      server.middlewares.use('/theme/dev', (req, res) => {
        const file = path.join(dir, decodeURIComponent((req.url ?? '/').split('?')[0]))
        const type = contentTypes[path.extname(file).toLowerCase()]
        if (!file.startsWith(dir + path.sep) || !type || !fs.statSync(file, { throwIfNoEntry: false })?.isFile()) {
          res.statusCode = 404
          res.end()
          return
        }
        res.setHeader('Content-Type', type)
        res.setHeader('Cache-Control', 'no-cache')
        res.end(fs.readFileSync(file))
      })
    },
  }
}
