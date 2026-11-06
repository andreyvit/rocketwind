// Run the original CLI canonicalization tests and capture their command calls.
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { spawnSync } from 'node:child_process'
const [upstream, tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/capture-cli.mjs UPSTREAM TOOLING')
const source = fs.realpathSync(path.resolve(upstream, 'packages/@tailwindcss-cli'))
const root = path.resolve('.cache/cli-tests')
const output = path.resolve('testdata/upstream/cli-canonicalize.json')
fs.cpSync(path.join(source,'src/commands/canonicalize/fixtures'),path.resolve('testdata/upstream/cli-canonicalize'),{recursive:true})
fs.mkdirSync(root, { recursive: true })
fs.writeFileSync(path.join(root, 'setup.ts'), `
import fs from 'node:fs'
import {afterAll,expect} from 'vitest'
const calls=[]
globalThis.__captureCLI=(kind,options,result)=>calls.push({test:expect.getState().currentTestName,kind,options,result})
afterAll(()=>fs.writeFileSync(${JSON.stringify(output)},JSON.stringify(calls,null,2)+'\\n'))
`)
fs.writeFileSync(path.join(root, 'vitest.config.mts'), `
import {defineConfig} from 'vitest/config'
export default defineConfig({
  root:${JSON.stringify(source)},
  define:{__VERSION__:JSON.stringify('4.3.4')},
  plugins:[{name:'capture-cli',enforce:'pre',transform(code,id){
    if (!id.endsWith('/commands/canonicalize/index.ts')) return
    code=code.replace('export async function runCommandLine(', 'async function originalRunCommandLine(')
      .replace('export async function streamStdin(', 'async function originalStreamStdin(')
    return code+\`
      import {Readable as CaptureReadable} from 'node:stream'
      export async function runCommandLine(options) {
        const result=await originalRunCommandLine(options)
        globalThis.__captureCLI('run',options,result)
        return result
      }
      export async function streamStdin(options) {
        let input='',output=''
        const source=options.input,target=options.output
        const wrapped=CaptureReadable.from((async function*(){for await(const chunk of source){input+=chunk.toString();yield chunk}})())
        await originalStreamStdin({...options,input:wrapped,output:{write(chunk,...rest){output+=chunk.toString();return target.write(chunk,...rest)}}})
        globalThis.__captureCLI('stream',{css:options.css,cwd:options.cwd,format:options.format,input},{stdout:output,stderr:'',exitCode:0})
      }
    \`
  }}],
  test:{include:['src/commands/canonicalize/canonicalize.test.ts'],setupFiles:[${JSON.stringify(path.join(root,'setup.ts'))}]},
})
`)
const require = createRequire(path.resolve(tooling, 'package.json'))
const cli = path.join(path.dirname(require.resolve('vitest/package.json')), 'vitest.mjs')
const result = spawnSync(process.execPath, [cli, 'run', '--config', path.join(root, 'vitest.config.mts')], { stdio: 'inherit' })
process.exit(result.status ?? 1)
