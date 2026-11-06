// Run all original tests for the three small plugins with their legacy
// Tailwind dependencies. Retain their inputs and CSS for the legacy profile.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {spawnSync} from 'node:child_process'
const [upstream,tooling,legacyTooling] = process.argv.slice(2)
if (!legacyTooling) throw new Error('usage: node scripts/capture-standard-plugin-tests.mjs UPSTREAM TOOLING LEGACY_TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
const legacyRequire = createRequire(path.resolve(legacyTooling,'package.json'))
const ts = require('typescript')
for (const name of ['aspect-ratio','line-clamp','container-queries']) {
  const root=path.join(upstream,name), cache=path.resolve('.cache',name+'-upstream'), output=path.resolve('testdata/plugins',name)
  fs.mkdirSync(cache,{recursive:true})
  for (const entry of ['src','tests','jest','package.json']) if(fs.existsSync(path.join(root,entry))) fs.cpSync(path.join(root,entry),path.join(cache,entry),{recursive:true})
  if(!fs.existsSync(path.join(cache,'node_modules'))) fs.symlinkSync(path.resolve(legacyTooling,'node_modules'),path.join(cache,'node_modules'))
  const testFile=name==='aspect-ratio'?'tests/test.js':name==='line-clamp'?'src/index.test.js':'tests/index.test.ts'
  fs.copyFileSync(path.join(root,testFile),path.join(output,path.basename(testFile)))
  const setup=path.join(cache,'capture.js')
  fs.writeFileSync(setup,`
const records=[];
globalThis.__rwRecord=(config,input,result)=>records.push(JSON.parse(JSON.stringify({name:expect.getState().currentTestName,config,input,result},(key,value)=>key==='plugins'?undefined:value)));
globalThis.__rwTailwind=original=>(config)=>{globalThis.__rwConfig=config;return original(config)};
afterAll(()=>require('fs').writeFileSync(${JSON.stringify(path.join(output,'upstream.json'))},JSON.stringify(records,null,2)+'\\n'));
`)
  if(name==='aspect-ratio') {
    let test=fs.readFileSync(path.join(cache,testFile),'utf8')
    test=test.replaceAll("require('tailwindcss')({","globalThis.__rwTailwind(require('tailwindcss'))({")
    test=test.replaceAll("require('tailwindcss-v2')({","globalThis.__rwTailwind(require('tailwindcss-v2'))({")
    test=test.replaceAll('expect(css).toBe(',"globalThis.__rwRecord(globalThis.__rwConfig,'@tailwind components',css); expect(css).toBe(")
    // The original v2 test forgets to return its promise. Await its assertion.
    test=test.replace("it('v2', () => {\n  postcss(","it('v2', () => {\n  return postcss(")
    fs.writeFileSync(path.join(cache,testFile),test)
  } else {
    const helper=name==='line-clamp'?'src/index.test.js':'tests/run.ts'
    let text=fs.readFileSync(path.join(cache,helper),'utf8')
    text=text.replace('return postcss(plugin(config)).process(', 'const result = await postcss(plugin(config)).process(')
    text=text.replace('function run(', 'async function run(')
    text=text.replace(/(from: [^\n]+\n\s*}\))/,`$1\n  globalThis.__rwRecord(config,${name==='line-clamp'?JSON.stringify('@tailwind utilities'):'input'},result.css)\n  return result`)
    fs.writeFileSync(path.join(cache,helper),text)
  }
  if(name==='container-queries') {
    for (const folder of ['src','tests']) for(const entry of fs.readdirSync(path.join(cache,folder))) {
      if(!entry.endsWith('.ts')) continue
      const file=path.join(cache,folder,entry)
      const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,esModuleInterop:true}})
      fs.writeFileSync(file.slice(0,-3)+'.js',compiled.outputText)
    }
  }
  const matchers=name==='line-clamp'?'jest/customMatchers.js':name==='container-queries'?'jest/custom-matchers.js':null
  const config={rootDir:cache,testMatch:[path.join(cache,testFile.replace(/\.ts$/,'.js'))],setupFilesAfterEnv:[...(matchers?[path.join(cache,matchers)]:[]),setup],transform:{},moduleNameMapper:{'^tailwindcss$':legacyRequire.resolve(name==='container-queries'?'tailwindcss-containers':'tailwindcss-line-clamp')}}
  const result=spawnSync(path.resolve(legacyTooling,'node_modules/.bin/jest'),['--runInBand','--config',JSON.stringify(config)],{cwd:cache,encoding:'utf8'})
  fs.writeFileSync(path.join(cache,'jest.log'),result.stdout+result.stderr)
  if(result.status!==0) throw new Error('Upstream '+name+' failed; see '+path.join(cache,'jest.log'))
  const records=JSON.parse(fs.readFileSync(path.join(output,'upstream.json')))
  if(records.length!==2)throw new Error('Expected 2 '+name+' fixtures, got '+records.length)
  console.log(`Passed both original ${name} tests`)
}
