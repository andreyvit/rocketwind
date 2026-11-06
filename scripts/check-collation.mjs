// Capture String.localeCompare results from the pinned Tailwind test runtime.
// Run after scripts/collation.py. Node and ICU are development tools only.
import fs from 'node:fs/promises'
import path from 'node:path'
import zlib from 'node:zlib'
import crypto from 'node:crypto'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const target = path.join(root, 'testdata/collation')
if (process.versions.icu !== '78.3' || process.versions.unicode !== '17.0') {
  throw new Error('Use Node with ICU 78.3 and Unicode 17.0, matching the reference runtime')
}
const source = 'https://raw.githubusercontent.com/unicode-org/cldr/release-48/common/uca/CollationTest_CLDR_NON_IGNORABLE_SHORT.txt'
const bytes = process.argv[2]
  ? await fs.readFile(process.argv[2])
  : Buffer.from(await (await fetch(source)).arrayBuffer())
const inputs = bytes.toString().split('\n').map(s => s.split('#')[0].trim()).filter(Boolean)
  .map(s => String.fromCodePoint(...s.split(/\s+/).map(v => parseInt(v, 16))))
  // Go strings contain UTF-8; replace lone UTF-16 surrogates just as Node's
  // UTF-8 encoder does. Keep every conformance input in the comparison corpus.
  .map(s => Buffer.from(s).toString())
const expected = Buffer.alloc(inputs.length - 1)
for (let i = 1; i < inputs.length; i++) {
  expected[i - 1] = Math.sign(inputs[i - 1].localeCompare(inputs[i])) + 1
}

const cases = []
const compare = (A, B) => cases.push({ A, B, Want: Math.sign(A.localeCompare(B)) })
const characters = [...Array.from({ length: 95 }, (_, i) => String.fromCharCode(i + 32)),
  'é', 'e\u0301', 'ß', 'ss', 'æ', 'œ', '中', '日', 'あ', 'ア', 'ა', 'Я', 'α',
  'İ', 'ı', '😀', '🤖', '🫨', '🪿', '🫩', '👨‍👩‍👧', '\u{10ffff}', '\u0000']
for (const a of characters) for (const b of characters) compare(a, b)

// Exercise contraction contexts, normalization and discontiguous matching.
const table = await fs.readFile(path.join(root, 'data/collation/unicode.bin'))
const count = table.readUInt32LE(8), pool = table.readUInt32LE(12)
const start = 16 + table.readUInt32LE(0) * 8 + table.readUInt32LE(4) * 12
for (let i = 0; i < count; i++) {
  const offset = pool + table.readUInt32LE(start + i * 12)
  const key = table.toString('utf8', offset, offset + table.readUInt32LE(start + i * 12 + 4))
  const runes = [...key]
  const variants = [key, key.normalize('NFC'), key.normalize('NFD'), runes.join('\u0000'),
    runes.join('\u0334'), runes.join('\u0323'), runes.join('\u0301'), runes.join('a')]
  for (const a of variants) for (const b of variants) compare(a, b)
}

await fs.mkdir(target, { recursive: true })
await fs.writeFile(path.join(target, 'cldr-48.txt.gz'), zlib.gzipSync(bytes))
await fs.writeFile(path.join(target, 'cldr-node.bin.gz'), zlib.gzipSync(expected))
await fs.writeFile(path.join(target, 'node-26.json.gz'), zlib.gzipSync(JSON.stringify(cases)))
await fs.copyFile(path.join(root, 'data/collation/LICENSE'), path.join(target, 'LICENSE'))
await fs.writeFile(path.join(target, 'manifest.json'), JSON.stringify({
  source, sha256: crypto.createHash('sha256').update(bytes).digest('hex'),
  node: process.version, icu: process.versions.icu, cldr: process.versions.cldr,
  unicode: process.versions.unicode, locale: new Intl.Collator().resolvedOptions(),
  conformance: inputs.length, comparisons: cases.length,
}, null, 2) + '\n')
console.log(`${inputs.length} CLDR inputs and ${cases.length} comparison pairs`)
