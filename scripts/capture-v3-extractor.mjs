import fs from 'node:fs'
import path from 'node:path'
import vm from 'node:vm'
import Module, {createRequire} from 'node:module'
import zlib from 'node:zlib'
const [tooling,upstream]=process.argv.slice(2)
const require=createRequire(path.resolve(tooling,'package.json'))
if(require('tailwindcss/package.json').version!=='3.4.17')throw new Error('expected 3.4.17')
const file=require.resolve('tailwindcss/lib/lib/defaultExtractor'),mod=new Module(file)
mod.filename=file;mod.paths=Module._nodeModulePaths(path.dirname(file))
mod._compile(fs.readFileSync(file,'utf8')+'\nexports.patterns=buildRegExps;',file)
const patterns=[...mod.exports.patterns({tailwindConfig:{prefix:'',separator:':'}})].map(r=>r.source)
fs.writeFileSync('v3_extract_generated.go','// Code generated from Tailwind CSS v3.4.17 defaultExtractor; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.\npackage rocketwind\n\nvar v3ExtractPatterns = []string{'+patterns.map(JSON.stringify).join(',')+'}\n')
const tests=[],fixtures=[];let before=()=>{},name=''
const test=(title,fn)=>tests.push([title,fn])
test.each=cases=>(title,fn)=>cases.forEach((args,i)=>test(title.replace('%#',i),()=>fn(...args)))
const original=fs.readFileSync(path.join(upstream,'tests/default-extractor.test.js'),'utf8')
fs.copyFileSync(path.join(upstream,'tests/default-extractor.test.js'),'testdata/v3/default-extractor.test.js')
vm.runInNewContext(original.replace(/^import .*\n/gm,''),{test,it:test,beforeEach:fn=>before=fn,expect:require('expect').expect,html:String.raw,createDefaultExtractor:context=>{const extract=mod.exports.defaultExtractor(context);return input=>{const result=extract(input);fixtures.push({name,input,results:[...new Set(result)].sort()});return result}}})
for(const [title,fn] of tests){name=title;before();await fn()}
fs.writeFileSync('testdata/v3/extractor.json.gz',zlib.gzipSync(JSON.stringify(fixtures)))
console.log(tests.length+' original tests passed; '+fixtures.length+' extraction calls')
