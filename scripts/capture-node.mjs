// Run original file-loading helper tests with a recording wrapper.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {spawnSync} from 'node:child_process'
const [upstream,tooling]=process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/capture-node.mjs UPSTREAM TOOLING')
const root=path.resolve('.cache/node-tests')
fs.mkdirSync(root,{recursive:true})
const source=path.resolve(upstream,'packages/@tailwindcss-node/src/urls')
const output=path.resolve('testdata/upstream/urls.json')
const original=fs.readFileSync(source+'.test.ts','utf8').replace("from './urls'",'from '+JSON.stringify(source))
fs.writeFileSync(path.join(root,'urls.test.ts'),`
import fs from 'node:fs'
import {vi,afterAll} from 'vitest'
const records=[]
vi.mock(${JSON.stringify(source)},async(importOriginal)=>{
  const module=await importOriginal()
  return {...module,rewriteUrls:async(args)=>{
    const result=await module.rewriteUrls(args)
    records.push({...args,result})
    return result
  }}
})
afterAll(()=>fs.writeFileSync(${JSON.stringify(output)},JSON.stringify(records,null,2)+'\\n'))
${original}
`)
fs.writeFileSync(path.join(root,'vitest.config.mts'),`import {defineConfig} from 'vitest/config'; export default defineConfig({root:${JSON.stringify(root)},test:{include:['*.test.ts']}})`)
const require=createRequire(path.resolve(tooling,'package.json'))
const cli=path.join(path.dirname(require.resolve('vitest/package.json')),'vitest.mjs')
const result=spawnSync(process.execPath,[cli,'run','--config',path.join(root,'vitest.config.mts')],{stdio:'inherit'})
process.exit(result.status??1)
