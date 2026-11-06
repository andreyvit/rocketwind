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
const {merge,castArray} = load(path.resolve(upstream,'src/utils.js'))
const q = JSON.stringify
function nodes(object) {
  const result = []
  for (const [key,value] of Object.entries(object)) {
    if (value == null) continue
    if (typeof value === 'object' && !Array.isArray(value)) {
      if (key.startsWith('@')) {
        const [,name,params=''] = key.trim().match(/^(@[^\s]+)(?:\s+(.*))?$/)
        result.push(`{Kind:"at-rule",Name:${q(name)},Value:${q(params)},Nodes:${nodes(value)}}`)
      } else result.push(`{Kind:"rule",Name:${q(key)},Nodes:${nodes(value)}}`)
    } else {
      const name = key.startsWith('--') ? key : key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase())
      for (const v of Array.isArray(value) ? value : [value]) result.push(`{Kind:"declaration",Name:${q(name)},Value:${q(String(v))}}`)
    }
  }
  return '[]*Node{'+result.join(',')+'}'
}
let go = '// Code generated from @tailwindcss/typography v0.5.20 by scripts/typography.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/plugins/typography/LICENSE.\npackage rocketwind\n\nvar typographyStyles = []TypographyStyle{\n'
for (const [name,config] of Object.entries(styles)) go += `{Name:${q(name)},Nodes:${nodes(merge({},...castArray(config.css || {})))}},\n`
go += '}\n'
fs.writeFileSync('typography_generated.go',go)
const target = 'testdata/plugins/typography'
fs.mkdirSync(target,{recursive:true})
fs.copyFileSync(path.join(upstream,'LICENSE'),path.join(target,'LICENSE'))
fs.writeFileSync(path.join(target,'manifest.json'),JSON.stringify({repository:'https://github.com/tailwindlabs/tailwindcss-typography',tag:'v0.5.20',commit:execFileSync('git',['rev-parse','HEAD'],{cwd:upstream,encoding:'utf8'}).trim()},null,2)+'\n')
console.log(`Extracted ${Object.keys(styles).length} typography modifiers`)
