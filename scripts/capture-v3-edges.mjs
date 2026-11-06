// Execute original normalization/collapse tests and capture independent edge cases.
import fs from 'node:fs'
import path from 'node:path'
import vm from 'node:vm'
import {createRequire} from 'node:module'
const [tooling, upstream, optimizer] = process.argv.slice(2)
const require = createRequire(path.resolve(tooling, 'package.json'))
const lightning = createRequire(path.resolve(optimizer, 'package.json'))('lightningcss')
if (require('tailwindcss/package.json').version !== '3.4.17') throw Error('expected 3.4.17')
const postcss = require('postcss'), tailwind = require('tailwindcss'), expect = require('expect').expect
const format = s => lightning.transform({filename:'input.css',code:Buffer.from(s),minify:false,targets:{chrome:106<<16},drafts:{nesting:true,customMedia:true}}).code.toString()
expect.extend({toMatchFormattedCss(actual, expected) { return {pass:format(actual)===format(expected),message:()=>format(actual)+'\n!=\n'+format(expected)} }})
const result={normalize:[],sheets:[],pseudos:[]}
let name = '', tests=[]
const it=(title,fn)=>tests.push([title,fn])
it.each=rows=>(title,fn)=>rows.forEach((row,i)=>it(title+' '+i,()=>fn(...row)))
const normalize = require('tailwindcss/lib/util/dataTypes').normalize
for (const filename of ['normalize-data-types.test.js','collapse-duplicate-declarations.test.js']) {
 const source=fs.readFileSync(path.join(upstream,'tests',filename),'utf8')
 fs.writeFileSync('testdata/v3/'+filename,source)
 tests=[]
 vm.runInNewContext(source.replace(/^import .*\n/gm,''),{it,expect,css:String.raw,html:String.raw,normalize:(input,options)=>{const output=normalize(input,options);result.normalize.push({input,output});return output},run:async(css,config)=>{const out=await postcss([tailwind(config)]).process(css,{from:undefined});result.sheets.push({name,css,config,classes:config.content.map(x=>x.raw).join(' '),result:out.css});return out}})
 for(const [title,fn] of tests){name=filename+': '+title;await fn()}
 console.log(filename+': '+tests.length+' original tests passed')
}
for(const input of ['calc([foo-bar]+2px)','calc(1+var(--foo','calc(1+theme(foo','x\\_y','--color','min(1+2)','calc(anchor-size(width)+1px)'])result.normalize.push({input,output:normalize(input)})
const parser=require('postcss-selector-parser'), move=require('tailwindcss/lib/util/pseudoElements').movePseudos
for(const input of ['.x', '.x::before:hover > .y', '.x::cue:hover > .y','.x::deep:hover .y','.x::part(foo):hover:focus','.x:is(::before,.y::part(a)):hover','.x:not(:is(::after)):hover','.x::selection:where(.y)', '.x::mystery:hover > .y::after','.x:has(::before,.y)::after']) {
 const ast=parser().astSync(input);ast.nodes.forEach(move);result.pseudos.push({input,output:ast.toString()})
}
fs.writeFileSync('testdata/v3/edges.json',JSON.stringify(result,null,2)+'\n')
