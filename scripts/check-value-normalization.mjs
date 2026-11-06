// Additional arithmetic and unit-boundary cases against the pinned source.
import fs from 'node:fs'
import path from 'node:path'
import Module, { createRequire } from 'node:module'
const [upstream, tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-value-normalization.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const ts = require('typescript')
Module._extensions['.ts'] = (mod, filename) => mod._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, filename)
const { canonicalizeCalcExpressions } = require(path.resolve(upstream, 'packages/tailwindcss/src/canonicalize-calc-expressions.ts'))
const { constantFoldDeclaration } = require(path.resolve(upstream, 'packages/tailwindcss/src/constant-fold-declaration.ts'))
const cases = []
function fold(input, rem = null, normalize = true) {
  cases.push({ kind: 'fold', input, rem, normalize, result: constantFoldDeclaration(input, rem, normalize) })
}
for (const unit of ['', 'px', '%', 'em', 'in', 'cm', 'mm', 'q', 'pc', 'pt', 'rem', 'grad', 'rad', 'turn', 'ms', 'kHz']) {
  for (const number of ['-0', '+0.0', '1', '-2.5', '0.0000001', '1000000000000000000000', '9'.repeat(400), '-' + '9'.repeat(400)]) {
    for (const normalize of [true, false]) fold(number + unit, 16, normalize)
  }
}
for (const input of [
  'calc(2 * var(--é))', 'calc(2em + 2px)', 'calc(2px + 2em)',
  'calc(var(--Á) + var(--a))', 'calc(var(--🫨) + var(--🤖))',
  'calc(1rem + 1)', 'calc(1 + 1rem)', 'calc(1cm * 1in)',
  'calc(' + '9'.repeat(400) + 'px - ' + '9'.repeat(400) + 'px)',
]) {
  cases.push({ kind: 'canonicalize', input, result: canonicalizeCalcExpressions(input) })
  fold(input, null, false)
}
fs.writeFileSync('testdata/upstream/value-normalization-extra.json', JSON.stringify(cases, null, 2) + '\n')
console.log(`${cases.length} additional normalization cases`)
