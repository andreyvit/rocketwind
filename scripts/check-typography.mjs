// Run the original plugin tests against their Tailwind 3 dependency, then
// compile the same inputs with Tailwind 4. Keep both outputs for conformance.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {execFileSync} from 'node:child_process'
const [upstream,tooling,legacyTooling] = process.argv.slice(2)
if (!legacyTooling) throw new Error('usage: node scripts/check-typography.mjs TYPOGRAPHY TOOLING V3_TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
const legacyVersion = createRequire(path.resolve(legacyTooling,'package.json'))('tailwindcss/package.json').version
if (legacyVersion !== '3.2.2') throw new Error(`Typography's original tests require Tailwind 3.2.2, found ${legacyVersion}`)
const cache = path.resolve('.cache/typography-upstream')
const output = path.resolve('testdata/plugins/typography')
fs.mkdirSync(path.join(cache,'src'),{recursive:true})
fs.mkdirSync(path.join(cache,'jest'),{recursive:true})
if (fs.existsSync(path.join(cache,'node_modules')) && fs.readlinkSync(path.join(cache,'node_modules')) !== path.resolve(legacyTooling,'node_modules')) fs.unlinkSync(path.join(cache,'node_modules'))
if (!fs.existsSync(path.join(cache,'node_modules'))) fs.symlinkSync(path.resolve(legacyTooling,'node_modules'),path.join(cache,'node_modules'))
for (const file of ['src/styles.js','src/utils.js','jest/customMatchers.js','package.json']) fs.copyFileSync(path.join(upstream,file),path.join(cache,file))
let pluginSource = fs.readFileSync(path.join(upstream,'src/index.js'),'utf8')
pluginSource += `\nconst original = module.exports; module.exports = Object.assign((options) => Object.assign(original(options), {__rocketwindOptions: options || {}}), original);\n`
fs.writeFileSync(path.join(cache,'src/index.js'),pluginSource)
let tests = fs.readFileSync(path.join(upstream,'src/index.test.js'),'utf8')
fs.copyFileSync(path.join(upstream,'src/index.test.js'),path.join(output,'index.test.js'))
tests = "const records = []; afterAll(() => require('fs').writeFileSync("+JSON.stringify(path.join(output,'upstream-v3.json'))+", JSON.stringify(records,null,2)+'\\n'));\n" + tests
tests = tests.replace('return postcss(plugin(config)).process(', 'const result = await postcss(plugin(config)).process(')
tests = tests.replace('function run(config, plugin = tailwind)', 'async function run(config, plugin = tailwind)')
tests = tests.replace("    }\n  )\n}", "    }\n  )\n  records.push({name:currentTestName,config:{...config,plugins:config.plugins.map(p=>p.__rocketwindOptions || {})},result:result.css});\n  return result\n}")
fs.writeFileSync(path.join(cache,'src/index.test.js'),tests)
try {
  const log = execFileSync(path.resolve(legacyTooling,'node_modules/.bin/jest'),['--runInBand','--rootDir',cache,'--config',path.join(cache,'package.json')],{encoding:'utf8',cwd:cache,stdio:['ignore','pipe','pipe']})
  fs.writeFileSync(path.join(cache,'jest.log'),log)
} catch (error) {
  fs.writeFileSync(path.join(cache,'jest.log'),String(error.stdout)+String(error.stderr))
  throw new Error('Original upstream tests failed; see '+path.join(cache,'jest.log'))
}
const original = JSON.parse(fs.readFileSync(path.join(output,'upstream-v3.json')))
if (original.length !== 16) throw new Error('missing upstream tests: '+original.length)
const {compile} = require('tailwindcss')
const typography = require('@tailwindcss/typography')
const legacyRequire = createRequire(path.join(cache,'package.json'))
const {merge,castArray,commonTrailingPseudos} = legacyRequire('./src/utils.js')
function nodes(object) {
  const result = []
  for (const [key,value] of Object.entries(object)) {
    if (value == null) continue
    if (typeof value === 'object' && !Array.isArray(value)) {
      if (key.startsWith('@')) {
        const [,name,params=''] = key.trim().match(/^(@[^\s]+)(?:\s+(.*))?$/)
        result.push({Kind:'at-rule',Name:name,Value:params,Nodes:nodes(value)})
      } else result.push({Kind:'rule',Name:key,Nodes:nodes(value)})
    } else {
      // Tailwind 4 preserves the property spelling for fallback arrays.
      const name = key.startsWith('--') || Array.isArray(value) ? key : key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase())
      for (const v of Array.isArray(value) ? value : [value]) result.push({Kind:'declaration',Name:name,Value:String(v)})
    }
  }
  return result
}
const styles = obj => obj === undefined ? null : Object.entries(obj).map(([Name,config])=>({Name,Nodes:nodes(merge({},...castArray(config.css || {})))}))
const cases = []
async function capture(name, css, candidates, config = {}, options = {}) {
  const result = await compile(css,{
    loadStylesheet:async(id)=>({content:fs.readFileSync(require.resolve(id),'utf8'),base:'',path:''}),
    // Content has already been extracted into candidates. Tailwind 4's config
    // adapter rejects v3 raw-content entries, unrelated to plugin behavior.
    loadModule:async(id)=>({module:id==='typography-config' ? {...config,plugins:[],content:[]} : typography,base:'',path:''}),
  })
  cases.push({name,css,candidates,config:{...config,plugins:[],content:[]},options:{ClassName:options.className || '',Legacy:options.target==='legacy',Styles:styles(config.theme?.typography),Extend:styles(config.theme?.extend?.typography)},result:result.build(candidates)})
}
for (const tt of original) {
  const config = tt.config
  const options = config.plugins[0]
  let css = '@import "tailwindcss/theme.css"; @plugin "@tailwindcss/typography"'
  css += Object.keys(options).length ? '{'+Object.entries(options).map(([k,v])=>`${k}: ${JSON.stringify(v)};`).join('')+'}' : ';'
  css += ' @config "typography-config"; @tailwind utilities;'
  const candidates = config.content.flatMap(x=>[...x.raw.matchAll(/class="([^"]+)"/g)].flatMap(x=>x[1].split(/\s+/)))
  await capture(tt.name,css,candidates,config,options)
}
const allClasses = Object.keys(typography().config.theme.typography).map(x=>x==='DEFAULT'?'prose':'prose-'+x)
for (const [name,options,prefix] of [['all defaults',{},''],['legacy',{target:'legacy'},''],['prefix',{},'tw'],['class name',{className:'markdown'},'']]) {
  let css = `@import "tailwindcss/theme.css"${prefix?' prefix(tw)':''}; @plugin "@tailwindcss/typography"`
  css += Object.keys(options).length ? '{'+Object.entries(options).map(([k,v])=>`${k}: ${JSON.stringify(v)};`).join('')+'}' : ';'
  css += ' @tailwind utilities;'
  let candidates = [...allClasses,'sm:prose','hover:prose-lg','prose-headings:font-bold','prose-lead:italic','prose-h5:text-sm','prose-invert!']
  candidates = candidates.map(c=>c.replaceAll('prose',options.className || 'prose')).map(c=>prefix?prefix+':'+c:c)
  await capture(name,css,candidates,{},options)
}
fs.writeFileSync(path.join(output,'fixtures.json'),JSON.stringify(cases,null,2)+'\n')
const selectors = ['p','p::before','p::marker::before','p::before, q::before','p::before, q::after','p::before, q','p, q::before','p::marker::before, q::before','p::before, q::marker::before','p::part(foo), q::part(bar)','p::before:hover','p::before q','[data-x="::before"]::after',':is(p,q)::before','p\\:x::after','p::before   ','p::before  , q::before','p::before + q::before','p::x::before, q::y::before','p::before::x, q::before::y']
fs.writeFileSync(path.join(output,'selectors.json'),JSON.stringify(selectors.map(input=>({input,result:commonTrailingPseudos(input)})),null,2)+'\n')
console.log(`Checked 16 original typography tests; captured ${cases.length} v4 compiler and ${selectors.length} selector cases`)
