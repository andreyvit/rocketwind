// Run from the repository root after installing the pinned reference packages.
// This measures independent builds, not an unchanged incremental-build cache.
// An optional third argument checks a Rocketwind CSS file against this output.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {performance} from 'node:perf_hooks'

const require = createRequire(path.resolve(process.argv[2] || '.cache', 'package.json'))
const {compile} = require('tailwindcss')
const {optimize} = require('@tailwindcss/node')
const themePath = require.resolve('tailwindcss/theme.css')
const theme = fs.readFileSync(themePath, 'utf8')
const modules = Object.fromEntries(['forms', 'typography'].map(name => [
  '@tailwindcss/' + name, require('@tailwindcss/' + name),
]))
const fixture = JSON.parse(fs.readFileSync('testdata/performance/loyalty.json', 'utf8'))
const times = []
let css
for (let i = 0; i < 120; i++) {
  const start = performance.now()
  const compiler = await compile(fixture.input, {
    loadStylesheet: async () => ({content: theme, base: '', path: themePath}),
    loadModule: async id => ({module: modules[id], base: '', path: id}),
  })
  css = optimize(compiler.build(fixture.classes), {minify: false}).code
  if (i >= 20) times.push(performance.now() - start)
}
times.sort((a, b) => a - b)
fs.mkdirSync('.cache/performance', {recursive: true})
fs.writeFileSync('.cache/performance/official-loyalty.css', css)
console.log(JSON.stringify({
  compiler: 'Tailwind CSS ' + require('tailwindcss/package.json').version,
  node: process.version,
  candidates: fixture.classes.length,
  iterations: times.length,
  medianMS: times[Math.floor(times.length / 2)],
  meanMS: times.reduce((a, b) => a + b) / times.length,
  p95MS: times[Math.floor(times.length * .95)],
  cssBytes: Buffer.byteLength(css),
}, null, 2))

if (process.argv[3]) {
  const {transform} = require('lightningcss')
  const postcss = require('postcss')
  function clean(value) {
    if (Array.isArray(value)) return value.map(clean).flatMap(item =>
      item?.type === 'style'
        ? item.value.selectors.map(selector => ({...item, value: {...item.value, selectors: [selector]}}))
        : [item])
    if (value && typeof value === 'object') return Object.fromEntries(
      Object.entries(value).filter(([key]) => key !== 'loc').map(([key, item]) => [key, clean(item)]))
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
  if (signature(fs.readFileSync(process.argv[3], 'utf8')) !== signature(css)) {
    throw Error('Rocketwind CSS differs from the reference')
  }
  console.log('Rocketwind CSS matches the reference, including rule order and prefixes.')
}
