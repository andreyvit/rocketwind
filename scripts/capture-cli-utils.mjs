// Capture the original CLI argument and diagnostic helper tests.
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { spawnSync } from 'node:child_process'
const [upstream,tooling]=process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/capture-cli-utils.mjs UPSTREAM TOOLING')
const source=fs.realpathSync(path.resolve(upstream,'packages/@tailwindcss-cli'))
const root=path.resolve('.cache/cli-utils'),output=path.resolve('testdata/upstream')
fs.mkdirSync(root,{recursive:true})
fs.writeFileSync(path.join(root,'setup.ts'),`
import fs from 'node:fs'
import {afterAll,expect} from 'vitest'
const calls=[]
globalThis.__captureCLIHelper=(kind,args,result,error)=>calls.push(JSON.parse(JSON.stringify({test:expect.getState().currentTestName,kind,args,result,error},(key,value)=>typeof value==='bigint'?Number(value):value)))
afterAll(()=>{
 const file=expect.getState().testPath.split('/').at(-1)
 fs.writeFileSync(${JSON.stringify(output)}+'/cli-'+file+'.json',JSON.stringify(calls,null,2)+'\\n')
})
`)
fs.writeFileSync(path.join(root,'vitest.config.mts'),`
import {defineConfig} from 'vitest/config'
export default defineConfig({root:${JSON.stringify(source)},
 plugins:[{name:'capture-cli-utils',enforce:'pre',transform(code,id){
  for(const [file,names] of Object.entries({'args':['args'],'format-ns':['formatNanoseconds'],'renderer':['relative','wordWrap']})) {
   if(!id.endsWith('/utils/'+file+'.ts'))continue
   for(const name of names) {
    code=code.replace(new RegExp('export function '+name+'(<[^>]*>)?\\\\('),'function original_'+name+'$1(')
    code+=\`
export function \${name}(...args) {
 const before=JSON.parse(JSON.stringify(args,(key,value)=>typeof value==='bigint'?Number(value):value))
 if ('\${name}'==='relative') {before[1]??=process.cwd();before[3]=process.cwd()}
 try { const result=original_\${name}(...args);globalThis.__captureCLIHelper('\${name}',before,result);return result }
 catch(error){globalThis.__captureCLIHelper('\${name}',before,undefined,error.message);throw error}
}
\`
   }
   return code
  }
 }}],test:{include:['src/utils/{args,format-ns,renderer}.test.ts'],setupFiles:[${JSON.stringify(path.join(root,'setup.ts'))}]}})
`)
const require=createRequire(path.resolve(tooling,'package.json'))
const cli=path.join(path.dirname(require.resolve('vitest/package.json')),'vitest.mjs')
const result=spawnSync(process.execPath,[cli,'run','--config',path.join(root,'vitest.config.mts')],{stdio:'inherit'})
process.exit(result.status??1)
