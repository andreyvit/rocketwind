// Extract declarative registrations from Tailwind 3 source, never test output.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'

const [tooling] = process.argv.slice(2)
if (!tooling) throw new Error('usage: node scripts/registry-v3.mjs TOOLING')
const require = createRequire(path.resolve(tooling, 'package.json'))
if (require('tailwindcss/package.json').version !== '3.4.17') throw new Error('expected Tailwind 3.4.17')
const {corePlugins} = require('tailwindcss/lib/corePlugins')
const resolveConfig = require('tailwindcss/resolveConfig')
const flatten = require('tailwindcss/lib/util/flattenColorPalette').default
const parseStyles = require('tailwindcss/lib/util/parseObjectStyles').default
const selectors = require('postcss-selector-parser')
const config = resolveConfig({content: []})
const marker = '__ROCKETWIND_VALUE__'
const result = {static: {}, functional: [], defaults: {}, manual: []}
const theme = (key, fallback) => key.split('.').reduce((v,k)=>v?.[k], config.theme) ?? fallback
let group = 0
function nodes(input) {
  return (input ?? []).filter(n => n.type !== 'comment' && !(n.type === 'atrule' && n.name === 'defaults')).map(n => {
    if(n.type === 'decl') return {kind:'declaration', name:n.prop, value:n.value, important:n.important || undefined}
    return {kind:n.type === 'atrule' ? 'at-rule' : 'rule', name:n.type === 'atrule' ? '@'+n.name : n.selector, value:n.params, nodes:nodes(n.nodes)}
  })
}
function addStatic(styles) {
  const order = group++
  for(const node of parseStyles(styles)) {
    const candidates = new Set()
    node.walkRules?.(n => selectors(sel => sel.walkClasses(c => candidates.add(c.value))).processSync(n.selector))
    if(node.type === 'rule') selectors(sel => sel.walkClasses(c => candidates.add(c.value))).processSync(node.selector)
    for(const name of candidates) {
      const clone = node.clone()
      const replace = n => {
        n.selector = selectors(sel => sel.walkClasses(c => {if(c.value === name) c.replaceWith(selectors.nesting({value:'&'}))})).processSync(n.selector)
      }
      clone.walkRules?.(replace)
      if(clone.type === 'rule') replace(clone)
      const ast = nodes([clone])
      const body = ast.length === 1 && ast[0].name === '&' ? ast[0].nodes : ast
      ;(result.static[name] ??= []).push({order, nodes:body})
    }
  }
}
for(const [plugin, run] of Object.entries(corePlugins)) {
  if(plugin === 'preflight' || plugin === 'container') continue
  let reads = []
  const api = {
    theme(key, fallback) { reads.push(key); return theme(key, fallback) },
    config(key, fallback) { return key === undefined ? config : key.split('.').reduce((v,k)=>v?.[k],config) ?? fallback },
    corePlugins: () => true,
    addUtilities: addStatic, addComponents: addStatic,
    addDefaults(name, declarations) { result.defaults[name] = nodes(parseStyles(declarations)) },
    matchUtilities(utilities, options = {}) {
      const order = group++
      for(const [name, fn] of Object.entries(utilities)) {
        const values = options.values ?? {}
        const keys = [...new Set([plugin, ...reads])].filter(k => {
          const source = theme(k)
          if(!source || typeof source !== 'object') return false
          const flat = flatten(source)
          return Object.entries(values).every(([key,value]) => JSON.stringify(source[key] ?? flat[key]) === JSON.stringify(value))
        })
        const namespace = keys[0] ?? ''
        if(!namespace && Object.keys(values).length) throw new Error('unknown theme namespace: '+plugin+'/'+name+' '+JSON.stringify(reads))
        const sample = fn(marker, {modifier:null})
        let compatible = sample != null
        for(const value of ['17.25px', '0', 'none', 'rgb(11 22 33)', 'var(--custom)', '3/7', 'calc(3px + 4rem)', '1px 2px 3px red']) {
          try { if(JSON.stringify(fn(value,{modifier:null})) !== JSON.stringify(sample)?.replaceAll(marker,value)) compatible = false }
          catch { compatible = false }
        }
        const type = [].concat(options.type ?? 'any').map(t => Array.isArray(t) ? t[0] : t)
        const definition = {name, namespace, order, type, negative:!!options.supportsNegativeValues,
          filterDefault:!!namespace && !Object.hasOwn(values,'DEFAULT') && Object.hasOwn(theme(namespace),'DEFAULT'),
          template:sample == null ? [] : nodes(parseStyles(sample)), compatible}
        result.functional.push(definition)
        if(!compatible || ['fontFamily','fontSize','animation','transitionProperty'].includes(namespace)) result.manual.push(plugin+'/'+name)
      }
    },
  }
  run(api)
}
fs.mkdirSync('.cache',{recursive:true})
fs.writeFileSync('.cache/registry-v3.json',JSON.stringify(result,null,2)+'\n')
console.log(JSON.stringify({static:Object.keys(result.static).length,functional:result.functional.length,manual:result.manual}))

const q=JSON.stringify
function goNodes(items) {
  return '[]*Node{'+items.map(n=>'{Kind:'+q(n.kind)+',Name:'+q(n.name)+
    (n.value !== undefined ? ',Value:'+q(n.value) : '')+(n.important ? ',Important:true' : '')+
    (n.nodes ? ',Nodes:'+goNodes(n.nodes) : '')+'}').join(',')+'}'
}
function goValue(value) {
  if(value === undefined || value === null) return 'nil'
  if(Array.isArray(value)) return '[]any{'+value.map(goValue).join(',')+'}'
  if(typeof value === 'function') {
    value=value({theme:key=>({['\u0000theme:'+key]:true}), colors:require('tailwindcss/colors'),
      breakpoints:()=>({'\u0000v3breakpoints':true})})
  }
  if(typeof value === 'object') {
    const entries=Object.entries(value)
    if(entries.length === 1 && entries[0][0].startsWith('\u0000theme:'))
      return 'ThemeFunc(func(t ThemeResolver) any { return t('+q(entries[0][0].slice(7))+') })'
    return 'ConfigObject{'+entries.map(([k,v])=>'{'+q(k)+','+goValue(v)+'}').join(',')+'}'
  }
  return typeof value === 'string' ? q(value) : String(value)
}
let go='// Code generated from Tailwind CSS v3.4.17 by scripts/registry-v3.mjs; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.\npackage rocketwind\n\n'
go+='var v3ThemeDefaults = '+goValue(require('tailwindcss/stubs/config.full').theme)+'\n'
const oldTypography = structuredClone(require('typography-v3/src/styles'))
for (const style of Object.values(oldTypography)) {
 const normalize = value => require('postcss-js').objectify(require('postcss').root({nodes:parseStyles(value)}))
 style.css = Array.isArray(style.css) ? style.css.map(normalize) : normalize(style.css)
}
go+='\nvar typographyLegacyTheme = '+goValue(oldTypography)+'\n'
go+='\nvar typographyV3Theme = '+goValue(require('@tailwindcss/typography/src/styles'))+'\n'
go+='\nvar v3Static = map[string][]v3StaticDefinition{\n'
for(const [name,defs] of Object.entries(result.static)) go+=q(name)+':{'+defs.map(d=>'{'+d.order+','+goNodes(d.nodes)+'}').join(',')+'},\n'
go+='}\n\nvar v3Functional = map[string][]v3UtilityDefinition{\n'
for(const [name,defs] of Map.groupBy(result.functional,d=>d.name))
  go+=q(name)+':{'+defs.map(d=>'{Namespace:'+q(d.namespace)+',Order:'+d.order+',Types:[]string{'+d.type.map(q).join(',')+'},Negative:'+d.negative+',FilterDefault:'+d.filterDefault+',Template:'+goNodes(d.template)+'}').join(',')+'},\n'
go+='}\n\nvar v3Completions = []v3CompletionDefinition{{"container",-1},'
const completionEntries=[]
for(const [name,defs] of Object.entries(result.static)) completionEntries.push({name,index:-1,order:defs[0].order})
for(const [name,defs] of Map.groupBy(result.functional,d=>d.name)) defs.forEach((d,index)=>completionEntries.push({name,index,order:d.order}))
completionEntries.sort((a,b)=>a.order-b.order)
go+=completionEntries.map(e=>'{'+q(e.name)+','+e.index+'}').join(',')
go+='}\n\nvar v3Defaults = '+goNodes(Object.values(result.defaults).flat())+'\n'
go+='\nvar v3ColorNames = map[string]string{'+Object.entries(require('tailwindcss/lib/util/colorNames').default).map(([k,v])=>q(k)+':'+q(v.join(' '))).join(',')+'}\n'
const context = require('tailwindcss/lib/lib/setupContextUtils').createContext(config)
go+='\nvar v3VariantFormats = map[string][]string{\n'
for(const variant of context.getVariants()) {
  if(variant.isArbitrary || Object.hasOwn(config.theme.screens,variant.name)) continue
  const formats=variant.name==='before' || variant.name==='after' ? ['&::'+variant.name] : variant.selectors()
  go+=q(variant.name)+':{'+formats.map(q).join(',')+'},\n'
}
go+='}\nvar v3VariantOrder = map[string]int{'+context.getVariants().map((v,i)=>q(v.name)+':'+i).join(',')+'}\n'
fs.writeFileSync('v3_registry_generated.go',go)
fs.mkdirSync('data/v3',{recursive:true})
fs.copyFileSync(require.resolve('tailwindcss/lib/css/preflight.css'),'data/v3/preflight.css')
