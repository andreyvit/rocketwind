// Independent reference results for the old-generation compatibility profile.
import fs from 'node:fs'
import path from 'node:path'
import zlib from 'node:zlib'
import {createRequire} from 'node:module'
const [tooling] = process.argv.slice(2)
if(!tooling) throw new Error('usage: node scripts/capture-v3.mjs TOOLING')
const require=createRequire(path.resolve(tooling,'package.json'))
if(require('tailwindcss/package.json').version !== '3.4.17') throw new Error('expected Tailwind 3.4.17')
const resolveConfig=require('tailwindcss/resolveConfig')
const {createContext}=require('tailwindcss/lib/lib/setupContextUtils')
const {generateRules}=require('tailwindcss/lib/lib/generateRules')
const postcss=require('postcss')
const context=createContext(resolveConfig({content:[]}))
const classes=context.getClassList()
const cases=[]
for(const candidate of classes) {
  const rules=generateRules(new Set([candidate]),context).map(([,node])=>node.clone())
  const root=postcss.root({nodes:rules})
  root.walkAtRules('defaults',n=>n.remove())
  cases.push({candidate,css:root.toString()})
}
fs.mkdirSync('testdata/v3',{recursive:true})
fs.writeFileSync('testdata/v3/utilities.json.gz',zlib.gzipSync(JSON.stringify(cases)))
console.log('Captured '+cases.length+' Tailwind 3.4.17 utility builds')
const variants=[]
for(const variant of context.variantMap.keys()) {
  for(const utility of ['flex','bg-black','text-black','border-black','content-["hi"]']) {
    const candidate=variant+':'+utility
    const root=postcss.root({nodes:generateRules(new Set([candidate]),context).map(([,node])=>node.clone())})
    root.walkAtRules('defaults',n=>n.remove())
    variants.push({candidate,css:root.toString()})
  }
}
for(const variant of ['group-[.x]/menu','group-[&:is(.x)]','aria-expanded:before','before:aria-expanded','hover:before','group-hover:before','[.x]','group-hover/menu','peer-checked/choice','has-[img]','group-has-[a]','peer-has-[:checked]','aria-[sort=ascending]','data-[state=open]','supports-[display:grid]','supports-[backdrop-filter]','min-[123px]','max-[456px]','[&>p]','sm:hover','hover:sm','group-hover:focus','before:hover']) {
  const candidate=variant+':bg-black'
  const root=postcss.root({nodes:generateRules(new Set([candidate]),context).map(([,node])=>node.clone())})
  root.walkAtRules('defaults',n=>n.remove())
  variants.push({candidate,css:root.toString()})
}
fs.writeFileSync('testdata/v3/variants.json',JSON.stringify(variants,null,2)+'\n')
console.log('Captured '+variants.length+' Tailwind 3.4.17 variant builds')
const configurations=[
  {name:'legacy arbitrary syntax',config:{},classes:['[name.dot:value]','[e.f(a:value]','[title::value]','[_property:value]','[-property:value]','[--foo:--bar]','[color:--brand]','[https://example.com]','bg-[--brand]','w-[calc(100%-theme(spacing.2))]','grid-cols-[minmax(min-content,{.widget.max-width.size})]','[display:flex]/2','[.x]:flex','[background:center,_no-repeat_{.image.position}/cover]']},

  {name:'prefix',config:{prefix:'tw-'},classes:['flex','tw-flex','hover:tw-bg-black','group-hover:tw-flex','peer-checked/item:tw-flex','-tw-mt-4','tw--mt-4','!tw-p-4','hover:!tw-p-4','tw-p-4!','[color:red]','tw-[color:red]','tw-bg-(--brand)']},
  {name:'important selector',config:{important:'#app'},classes:['flex','hover:flex','before:block','sm:flex','container','[color:red]']},
  {name:'important bool',config:{important:true},classes:['flex','hover:flex','before:block','container','!container','sm:container']},

  {name:'extended theme',config:{theme:{extend:{spacing:{custom:'17px'},colors:{brand:'#123456',alpha:'rgb(1 2 3 / <alpha-value>)',variable:'var(--brand)'},fontFamily:{custom:['Custom','serif'],feature:[['Feature','sans-serif'],{fontFeatureSettings:'"cv11"',fontVariationSettings:'"wght" 500'}]},fontSize:{custom:['19px',{lineHeight:'27px',letterSpacing:'0.1px',fontWeight:'600'}]},screens:{xs:'450px'},transitionDuration:{DEFAULT:'321ms'},transitionTimingFunction:{DEFAULT:'linear'},container:{center:true,padding:{DEFAULT:'12px',sm:'16px',xs:'8px'}}}}},classes:['p-custom','m-custom','-mt-custom','w-custom','h-custom','gap-custom','space-x-custom','translate-x-custom','bg-brand','bg-brand/50','text-brand','border-brand','ring-brand','from-brand','via-brand','to-brand','bg-alpha','bg-alpha/25','text-variable','bg-variable/50','font-custom','font-feature','text-custom','text-custom/6','text-custom/[1.5]','xs:flex','container','transition','transition-none']},
  {name:'replaced theme',config:{theme:{spacing:{x:'3px'},colors:{brand:'#abcdef'},fontSize:{custom:'2em'},screens:{tiny:'200px'},fontFamily:{sans:['Example','serif']},container:{padding:'10px'}}},classes:['p-x','p-4','bg-brand','bg-red-500','font-sans','text-custom','sm:flex','tiny:flex','container']},
]
for(const fixture of configurations) {
 const ctx=createContext(resolveConfig({content:[],...fixture.config}))
 fixture.results=fixture.classes.map(candidate=>{
  const root=postcss.root({nodes:generateRules(new Set([candidate]),ctx).map(([,node])=>node.clone())})
  root.walkAtRules('defaults',n=>n.remove())
  require('tailwindcss/lib/lib/evaluateTailwindFunctions').default(ctx)(root)
  return root.toString()
 })
}
fs.writeFileSync('testdata/v3/configuration.json',JSON.stringify(configurations,null,2)+'\n')
const sheets=[
  {name:'layer important',css:'@tailwind components; @tailwind utilities; @layer components { .card { @apply p-4 !important; } } @layer utilities { .heavy { @apply font-bold !important; } }',classes:['card','heavy']},
  {name:'layer screens',css:'@tailwind components; @layer components { @screen sm { .card { @apply flex; } } }',classes:['card']},
  {name:'always layer apply',css:'@tailwind utilities; @layer utilities { h1 { @apply text-xl; } }',classes:[]},
  {name:'important components',config:{important:'#app'},css:'@tailwind components; @tailwind utilities; @layer components { .card { @apply p-4; } } @layer utilities { .tab { tab-size: 4; } }',classes:['card','hover:card','!card','tab','hover:tab']},
  {name:'important bool components',config:{important:true},css:'@tailwind components; @tailwind utilities; @layer components { .card { @apply p-4; } } @layer utilities { .tab { tab-size: 4; } }',classes:['card','hover:card','!card','tab','hover:tab']},
  {name:'safelist',config:{safelist:['flex','hover:text-xl',{pattern:'^(bg-black|font-bold)$',variants:['hover','sm']}]},css:'@tailwind utilities;',classes:[]},
  {name:'explicit variants',css:'@tailwind variants; .after { color: red; } @tailwind utilities;',classes:['flex','hover:flex']},

  {name:'utilities without preflight',css:'@tailwind utilities;',classes:['flex','translate-x-2','text-red-500','shadow-sm']},
  {name:'base without utilities',css:'@tailwind base;',classes:[]},
  {name:'base and utilities',css:'@tailwind base; @tailwind components; @tailwind utilities;',classes:['flex','bg-black','before:block']},
  {name:'layers and apply',css:'@tailwind base; @tailwind components; @tailwind utilities; @layer components { .Card { @apply px-2 py-3 bg-white; } .Unused { @apply p-4; } .Card .title { color: red; } h1 { font-size: 3em; } } @layer utilities { .tab-4 { tab-size: 4; } } .external { @apply Card; margin: 1px; }',classes:['Card','tab-4','hover:Card','p-8']},
  {name:'screens and theme functions',css:'@tailwind utilities; .card { @apply text-xl/7 hover:text-red-500 sm:px-4; margin: theme(spacing.4); } @screen md { .card { @apply block; } }',classes:[]},
  {name:'invalid nested apply',css:'.card { @screen md { @apply block; } }',classes:[]},
  {name:'apply important',css:'@tailwind utilities; .card { @apply flex p-4 !important; }',classes:[]},
]
for(const sheet of sheets){try{const result=await postcss([require('tailwindcss')({content:[{raw:sheet.classes.join(' ')}],...sheet.config,safelist:sheet.config?.safelist?.map(v=>typeof v==='string'?v:{...v,pattern:new RegExp(v.pattern)})})]).process(sheet.css,{from:undefined});sheet.result=result.css}catch(error){sheet.error=error.reason??error.message}}
fs.writeFileSync('testdata/v3/stylesheets.json',JSON.stringify(sheets,null,2)+'\n')
const extra=new Set()
const registry=JSON.parse(fs.readFileSync('.cache/registry-v3.json'))
for(const {name,type,negative} of registry.functional){
  for(const value of ['17px','#123456','var(--x)','calc(100%_-_2rem)','13','0','50%','red','url(/x.png)','foo_bar','1fr,2fr','3/7']) {
    extra.add(name+'-['+value+']')
    if(negative) extra.add('-'+name+'-['+value+']')
  }
  for(const hint of type.filter(t=>t!=='any'&&t!=='lookup'))extra.add(name+'-['+hint+':var(--x)]')
}
const arbitrary=[]
for(const candidate of extra){const root=postcss.root({nodes:generateRules(new Set([candidate]),context).map(([,node])=>node.clone())});root.walkAtRules('defaults',n=>n.remove());arbitrary.push({candidate,css:root.toString()})}
fs.writeFileSync('testdata/v3/arbitrary.json.gz',zlib.gzipSync(JSON.stringify(arbitrary)))
console.log('Captured '+sheets.length+' stylesheet and '+arbitrary.length+' arbitrary-value builds')

const lists=[{}, {theme:{spacing:{x:'3px',auto:'auto'},colors:{brand:'#abc'}}}, {prefix:'tw-'}].map(config=>({config,classes:createContext(resolveConfig({content:[],...config})).getClassList({includeMetadata:true})}));
fs.writeFileSync('testdata/v3/completions.json.gz',zlib.gzipSync(JSON.stringify(lists)));
