// Development-only reference builds, using the pinned original Go-ported plugins.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
const require = createRequire(path.resolve(process.argv[2], 'package.json'))
if(require('tailwindcss/package.json').version !== '3.4.17') throw new Error('expected Tailwind 3.4.17')
const resolveConfig = require('tailwindcss/resolveConfig')
const {createContext} = require('tailwindcss/lib/lib/setupContextUtils')
const {generateRules} = require('tailwindcss/lib/lib/generateRules')
const postcss = require('postcss')
const cases=[]
for(const name of ['forms','typography','aspect-ratio','line-clamp','container-queries','forms@0.5.9','typography@0.5.15']) {
  const plugin=require(name==='forms@0.5.9'?'forms-v3':name==='typography@0.5.15'?'typography-v3':'@tailwindcss/'+name)
  for(const prefix of ['', 'tw-']) {
    const config={content:[],prefix,plugins:[plugin]}
    const ctx=createContext(resolveConfig(config))
    const roots={forms:['form-'],typography:['prose'], 'aspect-ratio':['aspect-w','aspect-h','aspect-none'], 'line-clamp':['line-clamp'], 'container-queries':['@container']}[name.split('@')[0]]
    let classes=[...new Set(ctx.getClassList().filter(c=>roots.some(root=>c.startsWith(prefix+root))))]
    classes.push(...classes.slice(0,2).map(c=>'hover:'+c))
    if(name==='typography') classes.push('prose-a:text-red-500'.replace('text-',prefix+'text-'))
    if(name==='container-queries') classes.push('@sm:'+prefix+'flex','@md/sidebar:'+prefix+'flex')
    for(const candidate of classes) {
      const compiled=await postcss([require('tailwindcss')({...config,content:[{raw:candidate}]})]).process('@tailwind components; @tailwind utilities;', {from:undefined})
      cases.push({name,prefix,candidate,result:compiled.css})
    }
  }
}
fs.writeFileSync('testdata/v3/plugins.json', JSON.stringify(cases,null,2)+'\n')
console.log('Captured '+cases.length+' original plugin builds')
