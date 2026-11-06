// Extract suggestion registrations, not test outputs, from pinned Tailwind.
import fs from 'node:fs'
import path from 'node:path'
import Module, { createRequire } from 'node:module'
const [upstream, tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/completions.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const ts = require('typescript')
const registrations = new Map()
globalThis.__rocketwindSuggest = (name, callback) => {
  if (!registrations.has(name)) registrations.set(name, [])
  registrations.get(name).push(callback)
}
Module._extensions['.ts'] = (mod, filename) => {
  let source = fs.readFileSync(filename, 'utf8')
  if (filename.endsWith('/utilities.ts')) source = source.replace(
    'function suggest(classRoot: string, defns: () => SuggestionDefinition[]) {',
    'function suggest(classRoot: string, defns: () => SuggestionDefinition[]) { globalThis.__rocketwindSuggest(classRoot,defns);',
  )
  mod._compile(ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText, filename)
}
const src = path.resolve(upstream, 'packages/tailwindcss/src')
const { Theme } = require(path.join(src, 'theme.ts'))
const theme = new Theme()
const utilities = require(path.join(src, 'utilities.ts')).createUtilities(theme)
require(path.join(src, 'compat/legacy-utilities.ts')).registerLegacyUtilities({ theme, utilities })
const staticNames = utilities.keys('static').filter(name => utilities.getCompletions(name).length > 0)
const roots = utilities.keys('functional')
const q = JSON.stringify
const strings = values => '[]string{' + (values ?? []).map(q).join(',') + '}'
const groups = values => '[]completionGroup{' + values.map(value => {
  if (typeof value === 'string') value = { values: [value] }
  const fields = []
  for (const [key, target] of Object.entries({ values: 'Values', modifiers: 'Modifiers', valueThemeKeys: 'ValueKeys', modifierThemeKeys: 'ModifierKeys' })) {
    if (value[key]?.length) fields.push(target + ':' + strings(value[key]))
  }
  for (const [key, target] of Object.entries({ supportsNegative: 'Negative', supportsFractions: 'Fractions', hasDefaultValue: 'Default' })) {
    if (value[key]) fields.push(target + ':true')
  }
  return '{' + fields.join(',') + '}'
}).join(',') + '}'
let go = '// Code generated from Tailwind CSS v4.3.4 by scripts/completions.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.\npackage rocketwind\n\n'
go += 'var builtinStaticCompletions=' + strings(staticNames) + '\n'
go += 'var builtinFunctionalCompletions=[]completionRoot{\n'
const originalGet = theme.get.bind(theme)
for (const root of roots) {
  if (!registrations.has(root)) continue
  go += '{Name:' + q(root) + ',Definitions:[]completionDefinition{'
  for (const callback of registrations.get(root)) {
    const keys = []
    theme.get = query => {
      if (query.length !== 1) throw new Error('multi-key condition: ' + query)
      if (!keys.includes(query[0])) keys.push(query[0])
      return null
    }
    callback()
    if (keys.length > 4) throw new Error('too many suggestion conditions')
    go += '{Keys:' + strings(keys) + ',Cases:[][]completionGroup{'
    for (let mask = 0; mask < (1 << keys.length); mask++) {
      theme.get = query => {
        const index = keys.indexOf(query[0])
        if (index < 0) throw new Error('conditional lookup changed')
        return (mask & (1 << index)) ? '1' : null
      }
      go += groups(callback()) + ','
    }
    go += '}},'
  }
  go += '}},\n'
}
theme.get = originalGet
go += '}\n'
const source = fs.readFileSync(path.join(src, 'utilities.ts'), 'utf8')
for (const [from, to] of [['DEFAULT_SPACING_SUGGESTIONS', 'spacingSuggestions'], ['suggestedFractions', 'fractionSuggestions']]) {
  const literal = source.match(new RegExp('(?:let|const) ' + from + ' = (\\[[\\s\\S]*?\\])'))[1]
  const values = [...literal.matchAll(/'([^']*)'/g)].map(m => m[1])
  go += 'var ' + to + '=' + strings(values) + '\n'
}
const variants = require(path.join(src, 'variants.ts')).createVariants(theme)
go += 'var builtinVariantCompletionOrder=' + strings([...variants.variants.keys()]) + '\n'
fs.writeFileSync('completions_generated.go', go)
console.log(`${staticNames.length} static and ${registrations.size} functional suggestion registrations`)
