// Check encoded maps independently with source-map-js, then retain extra edge cases.
import fs from 'node:fs'
import path from 'node:path'
import Module, {createRequire} from 'node:module'
const [upstream,tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-source-maps.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
process.env.NODE_ENV = 'test'
const {SourceMapGenerator} = require('source-map-js')
const ts = require('typescript')
Module._extensions['.ts'] = (mod,filename)=>mod._compile(ts.transpileModule(fs.readFileSync(filename,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText,filename)
const {compile} = require(path.resolve(upstream,'packages/tailwindcss/src/index.ts'))
function encode(decoded) {
  const generator = new SourceMapGenerator()
  const unknown = new Map()
  for (const mapping of decoded.mappings) {
    const source = mapping.originalPosition.source
    if (source.url === null && !unknown.has(source)) unknown.set(source, `<unknown ${unknown.size+1}>`)
    const name = source.url ?? unknown.get(source)
    generator.addMapping({source:name, original:mapping.originalPosition, generated:mapping.generatedPosition, name:mapping.name})
    generator.setSourceContent(name,source.content)
  }
  return JSON.parse(generator.toString())
}
const original = JSON.parse(fs.readFileSync('testdata/upstream/source-maps_source-map.test.ts.json','utf8'))
const encoded = original.filter(x=>x.kind==='compile.map').map(x=>({name:x.test,decoded:x.result,encoded:encode(x.result)}))
fs.writeFileSync('testdata/upstream/source-map-raw.json',JSON.stringify(encoded,null,2)+'\n')
const cases = [
  {name:'unicode',input:'@tailwind utilities; .😀 { content: "😃"; @apply text-[red]; }',candidates:['[--é:😀]','before:content-["😃"]']},
  {name:'CRLF and BOM',input:'\ufeff@tailwind utilities;\r\n.😀 {\r\n color: red;\r\n @apply flex;\r\n}',candidates:['grid']},
  {name:'apply token boundaries',input:'.card { @apply hover:flex flex; }',candidates:[]},
  {name:'apply whitespace',input:'.card { @apply  hover:flex\n flex; }',candidates:[]},
  {name:'custom variant',input:'@tailwind utilities; @custom-variant hocus { &:hover, &:focus { @slot; } }',candidates:['hocus:underline']},
  {name:'custom variant shorthand',input:'@tailwind utilities; @custom-variant mode (&:is(.mode *));',candidates:['mode:underline']},
  {name:'functional utility',input:'@tailwind utilities; @utility gapper-* { gap: --value(integer); }',candidates:['gapper-2']},
  {name:'properties',input:'@tailwind utilities;',candidates:['shadow-sm','ring-2','translate-x-2','font-bold']},
  {name:'custom properties',input:'@property --foo {syntax:"<length>";inherits:true;initial-value:0px} @property --bar {syntax:"<color>";inherits:false;initial-value:red} @tailwind utilities;',candidates:['flex']},
  {name:'untransformed input',input:'\n.foo { color: red; }\n',candidates:[]},
  {name:'empty source',input:'',candidates:[]},
  {name:'duplicate apply tokens',input:'.card { @apply flex grid flex; }',candidates:[]},
]
for (const test of cases) {
  const c=await compile(test.input,{from:'input.css'})
  test.css=c.build(test.candidates)
  test.decoded=c.buildSourceMap()
  test.encoded=encode(test.decoded)
}
fs.writeFileSync('testdata/upstream/source-map-extra.json',JSON.stringify(cases,null,2)+'\n')
console.log(`Encoded ${encoded.length} original maps and ${cases.length} additional cases`)
