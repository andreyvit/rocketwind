// Exercise the native optimizer used by the pinned Tailwind CLI against every
// distinct successful CSS result in the original core compiler fixtures.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {gzipSync} from 'node:zlib'
const tooling=process.argv[2]
if(!tooling)throw new Error('usage: node scripts/check-optimize.mjs TOOLING')
process.env.NODE_ENV='test'
const require=createRequire(path.resolve(tooling,'package.json'))
const {optimize}=require('@tailwindcss/node')
const lightning=JSON.parse(fs.readFileSync(path.join(path.dirname(require.resolve('lightningcss')),'../package.json'),'utf8'))
const originalWarn=console.warn
let warnings=[]
console.warn=(...args)=>warnings.push(args.join(' '))
const cases=new Map()
for(const file of fs.readdirSync('testdata/upstream').sort()){
 if(!file.endsWith('.test.ts.json'))continue
 const records=JSON.parse(fs.readFileSync(path.join('testdata/upstream',file),'utf8'))
 for(const record of records){
  if(record.kind!=='compile.build'||record.error||typeof record.result!=='string')continue
  if(cases.has(record.result))continue
  const entry={test:file+': '+record.test,css:record.result}
  warnings=[]
  try{
   entry.pretty=optimize(record.result,{minify:false}).code
   entry.compact=optimize(record.result,{minify:true}).code
  }catch(error){entry.error=error.message}
  if(warnings.length)entry.warnings=warnings
  cases.set(record.result,entry)
 }
}
console.warn=originalWarn
const output=JSON.stringify({tailwind:require('tailwindcss/package.json').version,lightningcss:lightning.version,cases:[...cases.values()]})+'\n'
fs.writeFileSync('testdata/upstream/optimize.json.gz',gzipSync(output))
console.log(`${cases.size} inputs, ${[...cases.values()].filter(c=>c.error).length} errors, ${output.length} bytes before compression`)
