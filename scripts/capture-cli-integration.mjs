// Run the original CLI integration suites against the pinned release and retain
// filesystem operations and assertions for the Go replay. JavaScript runs only
// in this development harness, never in Rocketwind.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {spawnSync} from 'node:child_process'
const [upstream,tooling]=process.argv.slice(2)
if(!tooling)throw Error('usage: node scripts/capture-cli-integration.mjs UPSTREAM TOOLING')
const source=fs.realpathSync(path.resolve(upstream))
const root=path.resolve('.cache/cli-integration-reference')
const output=path.resolve('testdata/upstream/cli-integration')
fs.mkdirSync(root,{recursive:true});fs.mkdirSync(output,{recursive:true})
const require=createRequire(path.resolve(tooling,'package.json'))
// npm reformats the distributed CSS. The original integration snapshots use
// the source checkout's CSS, as does the monorepo pack step. Keep the release's
// JavaScript and pack the exact pinned source stylesheets for this harness.
const packageRoot=path.join(root,'package')
fs.cpSync(path.dirname(require.resolve('tailwindcss/package.json')),packageRoot,{recursive:true})
for(const name of ['index.css','theme.css','preflight.css','utilities.css'])fs.copyFileSync(path.join(source,'packages/tailwindcss',name),path.join(packageRoot,name))
const packed=spawnSync('npm',['pack',packageRoot,'--ignore-scripts','--json','--pack-destination',root],{encoding:'utf8'})
if(packed.status!==0)throw Error(packed.stderr)
const tarball=path.join(root,JSON.parse(packed.stdout)[0].filename)
fs.cpSync(path.join(source,'integrations/cli'),path.join(root,'cli'),{recursive:true})
for(const file of fs.readdirSync(path.join(root,'cli'))){
 if(!file.endsWith('.test.ts'))continue
 const filename=path.join(root,'cli',file)
 const text=fs.readFileSync(filename,'utf8').replaceAll('../../packages/@tailwindcss-standalone/dist/',source+'/packages/@tailwindcss-standalone/dist/')
 fs.writeFileSync(filename,text)
}
let utils=fs.readFileSync(path.join(source,'integrations/utils.ts'),'utf8')
utils=utils.replace("const REPO_ROOT = path.join(__dirname, '..')",`const REPO_ROOT = ${JSON.stringify(source)}`)
 .replace(/const PUBLIC_PACKAGES = \(await fs.readdir\([\s\S]*?\n\)/,"const PUBLIC_PACKAGES = ['tailwindcss', '@tailwindcss/cli', '@tailwindcss/node', '@tailwindcss/oxide']")
 .replaceAll("from '../packages/",`from '${source}/packages/`)
 .replace(/function resolveVersion\(dependency: string\) \{[\s\S]*?\n\}/,`function resolveVersion(dependency: string) { return dependency==='tailwindcss'?${JSON.stringify('file:'+tarball)}:'4.3.4' }`)
 .replace("      config.fs['.gitignore'] ??=",`      const recording = recordIntegration(context, options.expect.getState().currentTestName, options.task.file.name)
      options.onTestFinished(() => recording.finish())
      config.fs['.gitignore'] ??=`)
utils="import {recordIntegration} from './recorder'\n"+utils
fs.writeFileSync(path.join(root,'utils.ts'),utils)
const recorder=fs.readFileSync('scripts/cli-integration-recorder.ts','utf8').replaceAll('__OUTPUT_DIRECTORY__',JSON.stringify(output))
fs.writeFileSync(path.join(root,'recorder.ts'),recorder)
fs.writeFileSync(path.join(root,'vitest.config.mts'),`
import {defineConfig} from 'vitest/config'
export default defineConfig({root:${JSON.stringify(root)},test:{include:['cli/*.test.ts'],maxWorkers:2,reporters:['default','json'],outputFile:{json:${JSON.stringify(path.join(root,'results.json'))}}}})
`)
const cli=path.join(path.dirname(require.resolve('vitest/package.json')),'vitest.mjs')
const result=spawnSync(process.execPath,[cli,'run','--config',path.join(root,'vitest.config.mts'),...process.argv.slice(4)],{stdio:'inherit'})
process.exit(result.status??1)
