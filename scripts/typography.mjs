// Extract declarative defaults from the pinned typography source, not snapshots.
import fs from 'node:fs'
import path from 'node:path'
import Module, {createRequire} from 'node:module'
import {execFileSync} from 'node:child_process'
const [upstream, tooling] = process.argv.slice(2)
if (!upstream || !tooling) throw new Error('usage: node scripts/typography.mjs TYPOGRAPHY TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
function load(file) {
  const mod = new Module(file)
  mod.filename = file
  mod.paths = [path.resolve(tooling,'node_modules')]
  mod.require = require
  mod._compile(fs.readFileSync(file,'utf8'),file)
  return mod.exports
}
const styles = load(path.resolve(upstream,'src/styles.js'))
const q = JSON.stringify
function object(value) {
  if (value === null) return 'nil'
  if (Array.isArray(value)) return '[]any{'+value.map(object).join(',')+'}'
  if (typeof value === 'object') return 'ConfigObject{'+Object.entries(value).map(([k,v])=>`{${q(k)},${object(v)}}`).join(',')+'}'
  return q(value)
}
let go = '// Code generated from @tailwindcss/typography v0.5.20 by scripts/typography.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/plugins/typography/LICENSE.\npackage rocketwind\n\nvar typographyTheme = '+object(styles)+'\n'
fs.writeFileSync('typography_generated.go',go)
const target = 'testdata/plugins/typography'
fs.mkdirSync(target,{recursive:true})
fs.copyFileSync(path.join(upstream,'LICENSE'),path.join(target,'LICENSE'))
fs.writeFileSync(path.join(target,'manifest.json'),JSON.stringify({repository:'https://github.com/tailwindlabs/tailwindcss-typography',tag:'v0.5.20',commit:execFileSync('git',['rev-parse','HEAD'],{cwd:upstream,encoding:'utf8'}).trim()},null,2)+'\n')
console.log(`Extracted ${Object.keys(styles).length} typography modifiers`)
