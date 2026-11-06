// Run from the repository root after installing the pinned reference packages.
// This measures independent builds, not an unchanged incremental-build cache.
// Pass a CSS filename after the tooling directory to check Rocketwind output.
// Add --compat=3 to use a tooling directory containing Tailwind 3.4.17.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {performance} from 'node:perf_hooks'

const compatibility = process.argv.includes('--compat=3') ? 3 : 4
const args = process.argv.slice(2).filter(arg => arg !== '--compat=3')
const require = createRequire(path.resolve(args[0] || '.cache', 'package.json'))
const version = require('tailwindcss/package.json').version
if (version !== (compatibility === 3 ? '3.4.17' : '4.3.4')) {
  throw Error(`Unexpected Tailwind version ${version} for v${compatibility}`)
}
const {optimize} = require('@tailwindcss/node')
const modules = Object.fromEntries(['forms', 'typography'].map(name => [
  '@tailwindcss/' + name, require('@tailwindcss/' + name),
]))
const fixture = JSON.parse(fs.readFileSync('testdata/performance/loyalty.json', 'utf8'))
let build
if (compatibility === 3) {
  const tailwind = require('tailwindcss')
  const postcss = require('postcss')
  const state = require('tailwindcss/lib/lib/sharedState')
  build = async () => {
    // A new PostCSS processor alone still reuses Tailwind's compilation context.
    // Drop contexts so each iteration configures plugins and builds from scratch.
    for (const key of ['contextMap', 'configContextMap', 'contextSourcesMap', 'sourceHashMap']) {
      state[key].clear()
    }
    const result = await postcss([tailwind({
      content: {
        files: [{raw: 'fixture', extension: 'html'}],
        extract: {html: line => line === 'fixture' ? fixture.classes : [line]},
      },
      plugins: [modules['@tailwindcss/forms']({strategy: 'class'}), modules['@tailwindcss/typography']],
    })]).process(fixture.inputV3, {from: undefined})
    return result.css
  }
} else {
  const {compile} = require('tailwindcss')
  const themePath = require.resolve('tailwindcss/theme.css')
  const theme = fs.readFileSync(themePath, 'utf8')
  build = async () => {
    const compiler = await compile(fixture.input, {
      loadStylesheet: async () => ({content: theme, base: '', path: themePath}),
      loadModule: async id => ({module: modules[id], base: '', path: id}),
    })
    return compiler.build(fixture.classes)
  }
}
const times = []
let css
for (let i = 0; i < 120; i++) {
  const start = performance.now()
  css = optimize(await build(), {minify: false}).code
  if (i >= 20) times.push(performance.now() - start)
}
times.sort((a, b) => a - b)
fs.mkdirSync('.cache/performance', {recursive: true})
fs.writeFileSync(`.cache/performance/official-loyalty${compatibility === 3 ? '-v3' : ''}.css`, css)
console.log(JSON.stringify({
  compiler: 'Tailwind CSS ' + version,
  node: process.version,
  candidates: fixture.classes.length,
  iterations: times.length,
  medianMS: times[Math.floor(times.length / 2)],
  meanMS: times.reduce((a, b) => a + b) / times.length,
  p95MS: times[Math.floor(times.length * .95)],
  cssBytes: Buffer.byteLength(css),
}, null, 2))

if (args[1]) {
  const {transform} = require('lightningcss')
  const postcss = require('postcss')
  function declarations(items) {
    // A single display declaration commutes with other properties, except all.
    // Keep fallback declarations and every other property's order intact.
    if (items.some(item => item.property === 'all')) return items
    const display = items.filter(item => item.property === 'display')
    return display.length === 1
      ? [...display, ...items.filter(item => item.property !== 'display')]
      : items
  }
  function clean(value) {
    if (Array.isArray(value)) {
      const items = value.map(clean).flatMap(item => item?.type === 'style'
        ? item.value.selectors.map(selector => ({...item, value: {...item.value, selectors: [selector]}}))
        : [item])
      const result = []
      for (const item of items) {
        const previous = result.at(-1)
        if (item?.type === 'style' && previous?.type === 'style' &&
            !item.value.rules?.length && !previous.value.rules?.length &&
            JSON.stringify({...item.value, declarations: null}) ===
            JSON.stringify({...previous.value, declarations: null})) {
          // Group expansion can reveal another pair of adjacent equal selectors.
          // Preserve declarations, including duplicate fallbacks and !important.
          previous.value.declarations = Object.fromEntries(
            Object.keys(previous.value.declarations).map(key => [key, declarations([
              ...previous.value.declarations[key], ...item.value.declarations[key],
            ])]))
        } else {
          result.push(item)
        }
      }
      return result
    }
    if (value && typeof value === 'object') return Object.fromEntries(
      Object.entries(value).filter(([key]) => key !== 'loc').map(([key, item]) => [key,
        (key === 'declarations' || key === 'importantDeclarations') && Array.isArray(item)
          ? declarations(item.map(clean)) : clean(item)]))
    return value
  }
  function signature(css) {
    const root = postcss.parse(css.replace(/\/\*! tailwindcss[\s\S]*?\*\//g, ''))
    // Lightning CSS conservatively keeps separate rules for unknown WebKit
    // pseudo-elements. Adjacent identical selectors can always be combined.
    root.walkRules(rule => {
      const previous = rule.prev()
      if (previous?.type === 'rule' && previous.selector === rule.selector) {
        previous.append(...rule.nodes)
        rule.remove()
      }
    })
    let code = Buffer.from(root.toString())
    // Normalize CSS spelling without browser targets: this must not repair a
    // missing prefix. Expand grouped selectors but preserve cascade order.
    for (let i = 0; i < 2; i++) code = transform({code, minify: true}).code
    let rules
    transform({code, visitor: {StyleSheetExit(sheet) { rules = clean(sheet.rules) }}})
    return JSON.stringify(rules)
  }
  if (signature(fs.readFileSync(args[1], 'utf8')) !== signature(css)) {
    throw Error('Rocketwind CSS differs from the reference')
  }
  console.log('Rocketwind CSS matches the reference, including rule order and prefixes.')
}
