// Pin how source-extracted CSS variables affect full and incremental builds.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
const [tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-references.mjs TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const {compile} = require('tailwindcss')
const theme = '@theme { --color-brand: red; --color-alias: var(--color-brand); --color-unused: blue; --animate-wave: wave 1s linear; @keyframes wave { to { opacity: 0 } } }'
const cases = [
  {name:'incremental variables and dependencies', css:theme+' @tailwind utilities;', steps:[[],['--unknown'],['--color-alias'],['flex'],['--animate-wave'],['--color-alias']]},
  {name:'escaped names', css:theme+' @tailwind utilities;', steps:[['--color-bran\\64 '],['--color-brand'],['--']]},
  {name:'prefix', css:theme.replace('@theme','@theme prefix(tw)')+' @tailwind utilities;', steps:[['--color-brand'],['--tw-color-brand'],['tw:flex'],['--tw-color-alias'],['--tw-animate-wave']]},
  {name:'prefix stripping', css:theme.replace('@theme','@theme prefix(tw)')+' @tailwind utilities;', steps:[['--'],['--xx-color-brand']]},
  {name:'inline theme', css:theme.replace('@theme','@theme inline')+' @tailwind utilities;', steps:[['text-brand'],['--color-brand']]},
  {name:'reference theme', css:theme.replace('@theme','@theme reference')+' @tailwind utilities;', steps:[['--color-brand'],['text-brand']]},
  {name:'excluded variable', css:theme+' @source not inline("--color-brand"); @tailwind utilities;', steps:[['--color-brand'],['--color-alias']]},
  {name:'static theme', css:theme.replace('@theme','@theme static')+' @tailwind utilities;', steps:[[],['--color-alias']]},
  {name:'no utilities', css:theme+' .card { display: block }', steps:[[],['--color-brand'],['flex']]},
  {name:'static input', css:'.card { display: block }', steps:[[],['--color-brand'],['flex']]},
  {name:'empty theme', css:'@tailwind utilities;', steps:[['--color-brand'],['flex']]},
]
for (const test of cases) {
  const compiler = await compile(test.css)
  test.results = test.steps.map(candidates => compiler.build(candidates))
}
fs.writeFileSync('testdata/upstream/references.json', JSON.stringify(cases,null,2)+'\n')
console.log(`Captured ${cases.length} variable-reference fixtures`)
