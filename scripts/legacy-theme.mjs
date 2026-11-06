// Convert upstream's declarative theme objects and theme references into Go.
// The seven bare-value functions have equivalent Go implementations.
import fs from 'node:fs'
import path from 'node:path'
import Module,{createRequire} from 'node:module'
const [upstream,tooling]=process.argv.slice(2)
if(!tooling)throw new Error('usage: node scripts/legacy-theme.mjs UPSTREAM TOOLING')
const require=createRequire(path.resolve(tooling,'package.json'))
const ts=require('typescript')
Module._extensions['.ts']=(mod,file)=>mod._compile(ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,esModuleInterop:true}}).outputText,file)
const defaults=require(path.resolve(upstream,'packages/tailwindcss/src/compat/default-theme.ts')).default
const q=JSON.stringify
function emit(value,key='') {
  if(typeof value==='function') {
    if(key==='__BARE_VALUE__') {
      const source=value.toString()
      const kind=source.includes('value.fraction')?'ratio':source.includes('minmax')?'repeat':source.includes('}ms')?'ms':source.includes('}px')?'px':source.includes('}deg')?'deg':source.includes('}%')?'%':'integer'
      return 'legacyBare('+q(kind)+')'
    }
    value=value({theme:key=>({['\u0000theme:'+key]:true})})
  }
  if(value==null)return 'nil'
  if(Array.isArray(value))return '[]any{'+value.map(v=>emit(v)).join(',')+'}'
  if(typeof value==='object')return 'ConfigObject{'+Object.entries(value).map(([k,v])=>'{'+q(k)+','+emit(v,k)+'}').join(',')+'}'
  return typeof value==='string'?q(value):String(value)
}
// createCompatConfig intentionally takes its palette only from CSS.
defaults.colors=()=>({'\u0000theme:color':true})
let code='// Code generated from Tailwind CSS v4.3.4 by scripts/legacy-theme.mjs; DO NOT EDIT.\n// MIT license: testdata/upstream/LICENSE.\npackage rocketwind\n\nvar legacyThemeDefaults = '+emit(defaults)+'\n'
fs.writeFileSync('legacy_theme_generated.go',code)
console.log(`Extracted ${Object.keys(defaults).length} legacy theme namespaces`)
