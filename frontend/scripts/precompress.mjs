// Precompress the build output so the server never spends CPU encoding it.
// Each compressible file gets .zst/.br/.gz siblings; internal/web serves the
// best one the client accepts and falls back to the plain file otherwise.
//
// Run as part of `npm run build`. Uses only node:zlib, so there is nothing
// to install; zstd is skipped on Node versions that lack it.
import { readdir, readFile, stat, writeFile } from 'node:fs/promises'
import path from 'node:path'
import zlib from 'node:zlib'

const DIST = path.resolve(import.meta.dirname, '..', 'dist')

// Extensions worth compressing. Anything else in dist (jpeg, png, woff2) is
// already compressed and would only grow.
const COMPRESSIBLE = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.map',
  '.mjs',
  '.svg',
  '.txt',
  '.webmanifest',
  '.xml',
])

// Below this the encoding overhead outweighs the saving, and the server's
// own threshold (minCompressSize in internal/api) matches.
const MIN_BYTES = 1024

const codecs = [
  {
    ext: '.zst',
    available: typeof zlib.zstdCompressSync === 'function',
    compress: (buf) =>
      zlib.zstdCompressSync(buf, {
        params: {
          [zlib.constants.ZSTD_c_compressionLevel]: 19,
        },
      }),
  },
  {
    ext: '.br',
    available: true,
    compress: (buf) =>
      zlib.brotliCompressSync(buf, {
        params: {
          [zlib.constants.BROTLI_PARAM_QUALITY]: 11,
          [zlib.constants.BROTLI_PARAM_SIZE_HINT]: buf.length,
        },
      }),
  },
  {
    ext: '.gz',
    available: true,
    compress: (buf) => zlib.gzipSync(buf, { level: 9 }),
  },
]

async function* walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(full)
    else if (entry.isFile()) yield full
  }
}

const skipped = codecs.filter((c) => !c.available).map((c) => c.ext)
if (skipped.length > 0) {
  console.log(`precompress: skipping ${skipped.join(', ')} (unsupported by this Node)`)
}

let files = 0
let raw = 0
const encoded = {}

for await (const file of walk(DIST)) {
  const ext = path.extname(file)
  if (!COMPRESSIBLE.has(ext)) continue
  const info = await stat(file)
  if (info.size < MIN_BYTES) continue
  const buf = await readFile(file)
  files += 1
  raw += buf.length
  for (const codec of codecs) {
    if (!codec.available) continue
    const out = codec.compress(buf)
    // A sibling that is not smaller would only cost the server a stat call.
    if (out.length >= buf.length) continue
    await writeFile(file + codec.ext, out)
    encoded[codec.ext] = (encoded[codec.ext] ?? 0) + out.length
  }
}

const kb = (n) => `${(n / 1024).toFixed(1)} kB`
const summary = Object.entries(encoded)
  .map(([ext, size]) => `${ext} ${kb(size)}`)
  .join(', ')
console.log(`precompress: ${files} files, ${kb(raw)} raw -> ${summary || 'nothing written'}`)
