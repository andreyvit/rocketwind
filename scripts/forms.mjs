// Extract the forms plugin's declarative rules from its pinned source.
// Theme and SVG operations remain placeholders, resolved by Go at runtime.
import fs from 'node:fs'
import path from 'node:path'
import Module, { createRequire } from 'node:module'
import { execFileSync } from 'node:child_process'

const [upstream, tooling] = process.argv.slice(2)
if (!upstream || !tooling) throw new Error('usage: node scripts/forms.mjs FORMS TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const file = path.resolve(upstream, 'src/index.js')
let source = fs.readFileSync(file, 'utf8')
source = source.replace("return color.replace('<alpha-value>', `var(${opacityVariableName}, 1)`)", 'return `RW_COLOR(${color}, ${opacityVariableName})`')
source = source.replace(/function resolveChevronColor\(color, fallback\) \{[\s\S]*?\n    \}/, 'function resolveChevronColor(color, fallback) { return `RW_CHEVRON(${color}, ${fallback})` }')
source = source.replace('const getStrategyRules =', 'globalThis.__rocketwindForms = rules; const getStrategyRules =')
const mod = new Module(file)
mod.filename = file
mod.paths = [path.resolve(tooling, 'node_modules')]
mod.require = (name) => name === 'mini-svg-data-uri' ? (svg) => 'RW_SVG:' + svg : require(name)
mod._compile(source, file)
mod.exports().handler({addBase(){},addComponents(){},theme:(key,fallback='')=>`RW_THEME(${key}, ${fallback})`})
if (!globalThis.__rocketwindForms) throw new Error('forms rules not captured')
const q = JSON.stringify
const kebab = (key) => key.startsWith('--') ? key : key.replace(/[A-Z]/g, c=>'-'+c.toLowerCase())
function nodesGo(object) {
  let nodes = []
  for (const [key, value] of Object.entries(object)) {
    if (value === null) continue
    if (typeof value === 'object' && !Array.isArray(value)) {
      if (key.startsWith('@')) {
        const [,name,params=''] = key.trim().match(/^(@[^\s]+)(?:\s+(.*))?$/)
        nodes.push(`{Kind:"at-rule",Name:${q(name)},Value:${q(params)},Nodes:${nodesGo(value)}}`)
      } else nodes.push(`{Kind:"rule",Name:${q(key)},Nodes:${nodesGo(value)}}`)
    } else for (const v of Array.isArray(value) ? value : [value]) nodes.push(`{Kind:"declaration",Name:${q(kebab(key))},Value:${q(String(v))}}`)
  }
  return '[]*Node{' + nodes.join(',') + '}'
}
let go = '// Code generated from @tailwindcss/forms v0.5.11 by scripts/forms.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/plugins/forms/LICENSE.\npackage rocketwind\n\n'
go += 'var formsRules = []formsRule{\n'
for (const rule of globalThis.__rocketwindForms) go += `{Base:[]string{${rule.base.map(q).join(',')}},Classes:[]string{${(rule.class??[]).map(q).join(',')}},Nodes:${nodesGo(rule.styles)}},\n`
go += '}\n'
const svgPackage = path.dirname(require.resolve('mini-svg-data-uri/package.json'))
const shortColors = require(path.join(svgPackage,'shorter-css-color-names.js'))
const hexColors = new Map()
for (const [name, pattern] of Object.entries(shortColors)) {
  const color = pattern.source.match(/#([0-9a-f]{6})/i)?.[1]
  if (!color) throw new Error('unhandled SVG color: ' + name)
  if (!hexColors.has(color)) hexColors.set(color,name)
}
go += '\nvar svgColorNames = map[string]string{\n' + [...hexColors].map(([hex,name])=>q(hex)+':'+q(name)+',').join('\n') + '\n}\n'
fs.writeFileSync('forms_generated.go',go)
const target = 'testdata/plugins/forms'
fs.mkdirSync(target,{recursive:true})
fs.copyFileSync(path.join(upstream,'LICENSE'),path.join(target,'LICENSE'))
fs.copyFileSync(path.join(svgPackage,'LICENSE'),path.join(target,'mini-svg-data-uri-LICENSE'))
fs.writeFileSync(path.join(target,'manifest.json'),JSON.stringify({repository:'https://github.com/tailwindlabs/tailwindcss-forms',tag:'v0.5.11',commit:execFileSync('git',['rev-parse','HEAD'],{cwd:upstream,encoding:'utf8'}).trim(),tests:'No automated test suite is present in this upstream release. Differential fixtures exercise strategies, helpers, variants, prefixes, important, colors and SVG fallback.'},null,2)+'\n')
console.log(`Extracted ${globalThis.__rocketwindForms.length} forms rules`)
