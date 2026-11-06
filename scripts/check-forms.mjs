// Capture differential checks against the real, pinned JavaScript plugin.
// The forms release has no automated upstream test suite to import.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
const [tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-forms.mjs TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
const {compile} = require('tailwindcss')
const forms = require('@tailwindcss/forms')
const svg = require('mini-svg-data-uri')
const candidates = ['form-input','form-textarea','form-select','form-multiselect','form-checkbox','form-radio','hover:form-input','focus:form-radio','sm:form-input']
const cases = []
for (const strategy of [undefined,'base','class','unknown','',null,['base','class']]) {
  for (const theme of ['', '--color-gray-500: rgb(10 20 30 / <alpha-value>); --color-blue-600: #f00;', '--color-gray-500: var(--gray); --color-blue-600: var(--blue);']) {
    const css = `@theme { --breakpoint-sm: 640px; ${theme} } @plugin "@tailwindcss/forms"${strategy === undefined ? ';' : `{ strategy: ${Array.isArray(strategy) ? strategy.map(x=>JSON.stringify(x)).join(', ') : JSON.stringify(strategy)}; }`} @tailwind utilities;`
    const compiled = await compile(css,{loadModule:async()=>({module:forms,base:'',path:''})})
    cases.push({strategy,theme,css,candidates,result:compiled.build(candidates)})
  }
}
for (const [prefix,important] of [['tw',false],['',true]]) {
  const css=`@theme${prefix ? ' prefix(tw)' : ''} { --breakpoint-sm: 640px; --color-gray-500: #0ffff; --color-blue-600: blue; } @plugin "@tailwindcss/forms"; @tailwind utilities${important?' important':''};`
  const classes=candidates.map(c=>prefix?prefix+':'+c:c)
  const compiled=await compile(css,{loadModule:async()=>({module:forms,base:'',path:''})})
  cases.push({css,candidates:classes,result:compiled.build(classes)})
}
{
  const css='@theme { --breakpoint-sm: 640px; } @plugin "@tailwindcss/forms"; @media important { @tailwind utilities; }'
  const compiled=await compile(css,{loadModule:async()=>({module:forms,base:'',path:''})})
  cases.push({css,candidates,result:compiled.build(candidates)})
}
fs.writeFileSync('testdata/plugins/forms/fixtures.json',JSON.stringify(cases,null,2)+'\n')
const colors=['#000','#000f','#000000','#000000ff','#FFF','#ffff','#ffffff','#ffffffff','#f00','#ff0000','#00f','#0ff','#0fff','#00ffffff','#ff000088','#ff0000zz','#f00_','\ufeff #abcdef \n\t<svg "quoted">é</svg>','rgb(10 20 30 / 1)','var(--custom)']
fs.writeFileSync('testdata/plugins/forms/svg.json',JSON.stringify(colors.map(input=>({input,result:svg(input)})),null,2)+'\n')
console.log(`Captured ${cases.length} forms compiler fixtures and ${colors.length} SVG cases`)
