// Extract declarative registrations, not test outputs, from pinned upstream code.
import fs from 'node:fs'
import path from 'node:path'
import Module, { createRequire } from 'node:module'

const [upstream, tooling] = process.argv.slice(2)
if (!upstream || !tooling) throw new Error('usage: node scripts/registry.mjs UPSTREAM TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
const ts = require('typescript')
const definitions = []
globalThis.__rocketwindDefinition = (kind, name, desc) => definitions.push({ kind, name, desc })
Module._extensions['.ts'] = (mod, filename) => {
  let source = fs.readFileSync(filename, 'utf8')
  if (filename.endsWith('/utilities.ts')) {
    source = source.replace('function functionalUtility(classRoot: string, desc: UtilityDescription) {',
      'function functionalUtility(classRoot: string, desc: UtilityDescription) { globalThis.__rocketwindDefinition("functional", classRoot, desc);')
    source = source.replace('function colorUtility(classRoot: string, desc: ColorUtilityDescription) {',
      'function colorUtility(classRoot: string, desc: ColorUtilityDescription) { globalThis.__rocketwindDefinition("color", classRoot, desc);')
  }
  mod._compile(ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText, filename)
}
const src = path.resolve(upstream, 'packages/tailwindcss/src')
const { Theme } = require(path.join(src, 'theme.ts'))
const { createUtilities } = require(path.join(src, 'utilities.ts'))
const theme = new Theme()
const defaults = {'--default-border-width':'1px','--default-outline-width':'1px','--default-ring-width':'1px','--default-ring-color':'currentcolor','--default-transition-timing-function':'ease','--default-transition-duration':'0s'}
for (const key of Object.keys(defaults)) theme.add(key,'__ROCKETWIND_DEFAULT_'+key.slice(2).replaceAll('-','_')+'__',1)
const utilities = createUtilities(theme)
require(path.join(src, 'compat/legacy-utilities.ts')).registerLegacyUtilities({theme, utilities})
const marker = '__ROCKETWIND_VALUE__'
const result = { static: {}, functional: [], manual: [] }
for (const name of utilities.keys('static')) {
  const nodes = utilities.get(name).filter(x => x.kind === 'static').flatMap(x => x.compileFn({kind:'static', root:name, variants:[], raw:name, important:false}) ?? [])
  result.static[name] = nodes
}
const captured = new Set()
for (const {kind, name, desc} of definitions) {
  captured.add(name)
  if (desc.supportsNegative) captured.add('-'+name)
  const {handle, handleBareValue, handleNegativeBareValue, ...meta} = desc
  const template = handle(marker, null)
  let compatible = template !== undefined
  for (const value of ['17.25px', 'rgb(11 22 33)', 'var(--custom)', '3/7', 'calc(3px + 4rem)']) {
    if (JSON.stringify(handle(value, null)) !== JSON.stringify(template)?.replaceAll(marker,value)) compatible = false
  }
  result.functional.push({kind,name,...meta,template,zeroTemplate:handle('0px',null),compatible,
    bare:handleBareValue?.toString(),negativeBare:handleNegativeBareValue?.toString()})
}
result.manual = utilities.keys('functional').filter(x => !captured.has(x))
fs.mkdirSync('.cache', {recursive:true})
fs.writeFileSync('.cache/registry.json', JSON.stringify(result,null,2)+'\n')
console.log(JSON.stringify({static:Object.keys(result.static).length,functional:result.functional.length,templates:result.functional.filter(x=>x.compatible).length,manual:result.manual.length}))

const q = JSON.stringify
function nodesGo(nodes) {
  return '[]*Node{' + (nodes??[]).map(n => {
    const fields = ['Kind:'+q(n.kind)]
    const name = n.selector ?? n.name ?? n.property
    const value = n.params ?? n.value
    if (name) fields.push('Name:'+q(name))
    if (value) fields.push('Value:'+q(value))
    if (n.important) fields.push('Important:true')
    if (n.nodes) fields.push('Nodes:'+nodesGo(n.nodes))
    return '{'+fields.join(',')+'}'
  }).join(',') + '}'
}
function bareKind(code) {
  if (!code) return ''
  if (code.includes('isValidSpacingMultiplier') && code.includes('--spacing(')) return 'spacing'
  if (code.includes('fraction')) return 'ratio'
  if (code.includes('repeat(')) return 'grid'
  if (code.includes("endsWith('%')")) return 'stretch'
  if (code.includes('valueAsNumber')) return 'angle'
  if (code.includes('isValidOpacityValue')) return 'opacity'
  if (code.includes('isPositiveInteger')) {
    for (const unit of ['px','deg','ms','%']) if(code.includes('}'+unit)) return 'integer'+unit
    return 'integer'
  }
  throw new Error('unhandled bare-value implementation: '+code)
}
let go = '// Code generated from Tailwind CSS v4.3.4 by scripts/registry.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.\npackage rocketwind\n\n'
go+='var builtinStatic = map[string][]*Node{\n'
for (const [name,nodes] of Object.entries(result.static)) go+=q(name)+':'+nodesGo(nodes)+',\n'
go+='}\n\nvar builtinFunctional = map[string][]utilityDefinition{\n'
const groups = Map.groupBy(result.functional,x=>x.name)
for(const [name,entries] of groups) {
  go+=q(name)+':{\n'
  for(const d of entries) {
    if(!d.compatible) throw new Error('nondeclarative handler: '+name)
    go+='{Color:'+String(d.kind==='color')+',Negative:'+String(!!d.supportsNegative)+',Fractions:'+String(!!d.supportsFractions)+','
    go+='Keys:[]string{'+(d.themeKeys??[]).map(q).join(',')+'},Bare:'+q(bareKind(d.bare))+','
    go+='HasDefault:'+String(Object.hasOwn(d,'defaultValue'))+',Default:'+q(d.defaultValue??'')+','
    go+='Template:'+nodesGo(d.template)+','
    if(JSON.stringify(d.zeroTemplate)!==JSON.stringify(d.template).replaceAll(marker,'0px')) go+='ZeroTemplate:'+nodesGo(d.zeroTemplate)+','
    if(d.staticValues) go+='Static:map[string][]*Node{'+Object.entries(d.staticValues).map(([k,v])=>q(k)+':'+nodesGo(v)).join(',')+'},'
    go+='},\n'
  }
  go+='},\n'
}
go+='}\n'
const order = require(path.join(src,'property-order.ts')).default
go+='\nvar propertyOrder=map[string]int{\n'+order.map((p,i)=>q(p)+':'+i+',').join('\n')+'\n}\n'
const {parse} = require(path.join(src,'css-parser.ts'))
const themeNodes = parse(fs.readFileSync(path.resolve(upstream,'packages/tailwindcss/theme.css'),'utf8'))
const themeVars = {}
const keyframes = []
for(const n of themeNodes) if(n.kind==='at-rule' && n.name==='@theme') {
  for(const child of n.nodes) {
    if(child.kind==='declaration') themeVars[child.property]=child.value
    else if(child.kind==='at-rule' && child.name==='@keyframes') keyframes.push(child)
  }
}
go+='\nvar builtinTheme=map[string]string{\n'+Object.entries(themeVars).map(([k,v])=>q(k)+':'+q(v)+',').join('\n')+'\n}\n'
go+='\nvar builtinKeyframes='+nodesGo(keyframes)+'\n'
const templates = []
const templateIDs = new Map()
go+='\nvar manualTemplateIndex=map[string]map[string]int{\n'
for(const name of result.manual) {
  go+=q(name)+':{'
  for(const type of ['default','','unknown','color','length','number','percentage','angle','vector','image','position','bg-size','generic-name','family-name','size','absolute-size','relative-size']) {
    const candidate = {kind:'functional', root:name, variants:[], raw:name, important:false, modifier:null, value:type==='default'?null:{kind:'arbitrary',value:marker,dataType:type||null}}
    const nodes = utilities.get(name).filter(x=>x.kind==='functional').flatMap(x=>x.compileFn(candidate)??[])
    if(!nodes.length) continue
    const key=JSON.stringify(nodes)
    if(!templateIDs.has(key)) { templateIDs.set(key,templates.length);templates.push(nodes) }
    go+=q(type)+':'+templateIDs.get(key)+','
  }
  go+='},\n'
}
go+='}\n\nvar manualTemplates=[][]*Node{\n'+templates.map(nodesGo).join(',\n')+',\n}\n'
const colorSource=fs.readFileSync(path.join(src,'utils/is-color.ts'),'utf8')
go+='\nvar namedColors=map[string]bool{\n'+[...new Set([...colorSource.matchAll(/^  '([^']+)',/gm)].map(x=>x[1]))].map(x=>q(x)+':true,').join('\n')+'\n}\n'
go+='\nvar defaultValues=map[string]string{\n'+Object.entries(defaults).map(([k,v])=>q(k)+':'+q(v)+',').join('\n')+'\n}\n'
const variants=require(path.join(src,'variants.ts')).createVariants(theme)
go+='\nvar builtinVariants=map[string]variantDefinition{\n'
for(const [name,v] of variants.variants) {
  let nodes=[]
  if(v.kind==='static') {const r={kind:'rule',selector:'&',nodes:[{kind:'slot'}]};v.applyFn(r,{kind:'static',root:name});nodes=r.nodes}
  go+=q(name)+':{Kind:'+q(v.kind)+',Order:'+v.order+',Compounds:'+v.compounds+',Nodes:'+nodesGo(nodes)+'},\n'
}
go+='}\n'
fs.writeFileSync('registry_generated.go',go)
