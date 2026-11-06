import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {execFileSync} from 'node:child_process'
const [upstream,tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/check-standard-plugins.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
const {compile} = require('tailwindcss')
const candidates = {
  'aspect-ratio':['aspect-none','aspect-w-1','aspect-w-2','aspect-w-16','aspect-h-2','aspect-w-[123]','aspect-w-[var(--width)]','aspect-h-[123]','aspect-h-[var(--height)]','aspect-w-special','aspect-h-special','aspect-w-video','aspect-w-17','aspect-w-2/no','sm:aspect-w-3','hover:aspect-none','aspect-w-2!'],
  'line-clamp':['line-clamp-1','line-clamp-6','line-clamp-7','line-clamp-seven','line-clamp-[33]','line-clamp-[var(--line-clamp-variable)]','line-clamp-none','hover:line-clamp-none','sm:line-clamp-2','line-clamp-2!','-line-clamp-2','line-clamp-2/no'],
  'container-queries':['@container','@container-normal','@container/sidebar','@container-normal/sidebar','@container-[size]/sidebar','@xs:underline','@sm:underline','@sm/container1:underline','@sm/container2:underline','@sm/container10:underline','@md:underline','@lg:underline','@lg/container1:underline','@[1024px]:underline','@[1024px]/container1:underline','@[1024]/container1:underline','@[312px]:underline','@[200rem]:underline','@[123px]:underline','@wide:underline','@[.5em]:underline','@[-1rem]:underline','@[calc(100px)]:underline','@[var(--size)]:underline','hover:@md:underline','@lg:@md:underline'],
}
for (const [name,classes] of Object.entries(candidates)) {
  const root = path.join(upstream,name)
  const target = path.join('testdata/plugins',name)
  fs.mkdirSync(target,{recursive:true})
  const hasLicenseFile=fs.existsSync(path.join(root,'LICENSE'))
  fs.copyFileSync(hasLicenseFile?path.join(root,'LICENSE'):'testdata/plugins/forms/LICENSE',path.join(target,'LICENSE'))
  let plugin = require('@tailwindcss/'+name)
  if (name==='line-clamp') {
    const handler = plugin.handler
    // v4 removed variants(); its old configuration merely selects generated
    // variants in v3. Supply the obsolete API while checking all emitted CSS.
    plugin = {...plugin,handler(api){return handler({...api,variants(){return []}})}}
  }
  const cases = []
  for (const prefix of ['', 'tw']) for (const theme of ['', '--aspect-special: 13; --aspect-2: 20; --line-clamp-seven: 7; --container-wide: 90rem; --container-md: 30rem;']) for (const defaults of [false,true]) {
    const css = `${defaults ? '@import "tailwindcss/theme.css"'+(prefix?' prefix(tw)':'')+';' : '@theme'+(prefix?' prefix(tw)':'')+' { --breakpoint-sm: 640px; }'} @theme { ${theme} } @plugin "@tailwindcss/${name}"; @tailwind utilities;`
    const compiled = await compile(css,{loadModule:async()=>({module:plugin,base:'',path:''}),loadStylesheet:async(id)=>({content:fs.readFileSync(require.resolve(id),'utf8'),base:'',path:''})})
    const candidates = classes.map(c=>prefix ? prefix+':'+c : c)
    cases.push({css,candidates,result:compiled.build(candidates)})
  }
  const original=JSON.parse(fs.readFileSync(path.join(target,'upstream.json')))
  for(const record of original) {
    const config={...record.config,content:[],plugins:[]}
    delete config.purge
    const content=record.config.content ?? record.config.purge?.content ?? []
    const candidates=content.flatMap(x=>x.raw.includes('class=')?[...x.raw.matchAll(/class="([^"]+)"/g)].flatMap(x=>x[1].split(/\s+/)):x.raw.split(/\s+/)).filter(Boolean)
    const css=`@plugin "@tailwindcss/${name}"; @config "fixture"; @tailwind utilities;`
    const compiled=await compile(css,{loadModule:async(id)=>({module:id==='fixture'?config:plugin,base:'',path:''})})
    const themeKey={'aspect-ratio':'aspectRatio','line-clamp':'lineClamp','container-queries':'containers'}[name]
    const values=config.theme?.[themeKey]
    cases.push({name:record.name,css,candidates,original:true,values:values?Object.entries(values).map(([Name,Value])=>({Name,Value})):null,result:compiled.build(candidates)})
  }
  fs.writeFileSync(path.join(target,'fixtures.json'),JSON.stringify(cases,null,2)+'\n')
  fs.writeFileSync(path.join(target,'manifest.json'),JSON.stringify({repository:'https://github.com/tailwindlabs/tailwindcss-'+name,version:require('@tailwindcss/'+name+'/package.json').version,commit:execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(),license:hasLicenseFile?'Original LICENSE file':'package.json declares MIT; this release omits a license file. Included the MIT text and Tailwind Labs attribution used by its other plugins.',developmentAdapter:name==='line-clamp'?'Tailwind 4 removed variants(); the differential harness supplies an empty variants function. All CSS is compared.':undefined},null,2)+'\n')
  console.log(`Captured ${cases.length} ${name} compiler fixtures`)
}
