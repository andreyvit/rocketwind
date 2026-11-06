// Additional canonicalization boundaries against the pinned Tailwind source.
import fs from 'node:fs'
import path from 'node:path'
import Module, { createRequire } from 'node:module'
const [upstream, tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-canonicalize.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const ts = require('typescript')
Module._extensions['.ts'] = (mod, filename) => mod._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText, filename)
const { __unstable__loadDesignSystem } = require(path.resolve(upstream, 'packages/tailwindcss/src/index.ts'))
const cases = []
const theme = `@tailwind utilities; @theme {
  --spacing: .25rem; --color-red: #f00; --color-blue: #00f; --color-label: var(--color-red);
  --text-sm: .875rem; --text-sm--line-height: 1.25rem; --breakpoint-sm: 40rem;
  --container-sm: 24rem; --opacity-50: .5; --setting-size: 12px;
}`
const groups = [
  ['p-4', 'm-2', 'unknown', 'unknown'],
  ['mt-1', 'mb-1', 'ml-2', 'mr-2'], ['text-sm', 'leading-7', 'leading-[28px]'],
  ['w-2', 'h-2', 'min-w-2', 'min-h-2'], ['w-2', 'h-3', 'size-4'],
  ['-mt-1', 'mb-1', 'ml-1', 'mr-1'], ['ps-1', 'pe-1'],
  ['before:content-["hello"]', 'after:content-["hello"]'],
  ['border-red/100'], ['text-[var(--color-red)]/100'], ['[color:var(--color-label)]'],
  ['w-[length:var(--foo)]'], ['w-[angle:var(--foo)]'], ['font-[number:var(--foo)]'],
  ['text-[color:var(--foo)]/[abc.%]'], ['text-red/[50.0%]'], ['text-red/[.5]'],
  ['text-[theme("colors.red")]'], ["text-[theme('colors.red')]"], ['text-[theme(_colors.red)]'],
  ['text-[theme( colors.red)]'], ['[width:calc(var(--container-sm)*1)]'],
  ['w-[calc(var(--setting-size)*1)]'],
  ['w-[calc(var(--setting-size)*2)]'], ['[width:calc(var(--setting-size)*1)]'],
  ['text-[theme("colors.red",theme("colors.blue"))]'],
  ['text-[theme("colors.a\\\"b")]'], ["text-[theme('colors.a\\\\b')]"],
  ['shadow-[0_0_theme(colors.red/50%)]'], ['w-[calc(1px*theme(spacing.4))]'],
  ['[color:theme(colors.red,red)]/[50%]'], ['text-[theme(colors.red/var(--a))]'],
  ['text-[theme(colors.red/50%/50%)]'], ['text-[theme(var(--foo))]'],
  ['grid-cols-[repeat(2,minmax(0,1fr))]'], ['aspect-[16/9]'], ['opacity-[50%]'],
  ['[--setting:12px]'], ['[color:var(--color-red,var(--missing))]'],
  ['has-group-hover/foo:flex'], ['not-group-hover/foo:flex'], ['in-group-hover/foo:flex'],
  ['group-hover/sidebar:flex'], ['has-[:hover]/foo:flex'],
  ['[&:hover,&:focus]:flex'], ['[&:is(:hover,:focus)]:flex'], ['[&:is([data-x],[data-y])]:flex'],
  ['[&:not(:hover,:focus)]:flex'], ['[&:not(:is(:hover))]:flex'],
  ['[&:nth-last-of-type(3)]:flex'], ['[&:nth-child(2n+1)]:flex'],
  ['[&:is(*:focus)]:flex'], ['[&:where(:focus)]:flex'], ['[&:not(*:focus)]:flex'],
  ['[&[aria-expanded=false]]:flex'], ['[&[aria-expanded~=true]]:flex'],
  ['[&[aria-expanded="false"_i]]:flex'], ['[&[1bad]]:flex'],
  ['[&[foo=]]:flex'], ['[&[foo?=bar]]:flex'],
  ['[@media_not_(width>10px)]:flex'], ['[@supports_not_(foo:bar)]:flex'],
  ['[&::part(foo)]:flex'], ['[&>_:first-line]:flex'],
  ['supports-[foo-bar]:flex'], ['aria-[foo*=true]:flex'], ['data-[x=y]:flex'],
  ['max-[--theme(--missing)]:flex'], ['[@media(width>--theme(--missing))]:flex'],
  ['[&:is()]:flex'], ['[&:not()]:flex'], ['[&]:flex'],
]
for (const options of [{}, { collapse: true, logicalToPhysical: true, rem: 16 }]) {
  const ds = await __unstable__loadDesignSystem(theme)
  for (const input of groups) cases.push({ css: theme, options, input, output: ds.canonicalizeCandidates(input, options) })
}
const custom = theme + `
@utility alias-a { width: var(--foo); }
@utility alias-b { width: var(--foo); }
@utility note { /*! retained */ --tw-sort: note; color: #F00; }
@utility multi { width: 4px; height: 4px; }
@utility p-1 { width: 4px; }
@utility trial-* { width: --value([length]); color: --modifier([color]); }
@utility same-leading { line-height: 24px; color: red; }
@utility a { width: 1px; }
@utility b { height: 1px; }
@utility c { color: red; }
@utility ab { width: 1px; height: 1px; }
@utility ac { width: 1px; color: red; }
@utility bc { height: 1px; color: red; }
@theme { --spacing-copy: 1px; }
@custom-variant nth (&:custom);
`
const ds = await __unstable__loadDesignSystem(custom)
for (const input of [['w-[length:var(--foo)]'], ['w-[angle:var(--foo)]'], ['note','text-red'], ['p-1','h-1','w-1'], ['alias-a','alias-b'], ['trial-[4px]/[red]'], ['text-sm','leading-6','same-leading'], ['m-[-1px]'], ['a','b','c'], ['[&:nth-child(3)]:flex']]) {
  const options = { collapse: true, logicalToPhysical: true, rem: 16 }
  cases.push({css: custom, options, input, output: ds.canonicalizeCandidates(input, options)})
}
fs.writeFileSync('testdata/upstream/canonicalize-extra.json', JSON.stringify(cases,null,2)+'\n')
console.log(`${cases.length} additional canonicalization cases`)
