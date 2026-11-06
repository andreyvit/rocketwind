// Additional parser and URL edge cases, evaluated by the pinned source.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {spawnSync} from 'node:child_process'
const [upstream, tooling] = process.argv.slice(2)
if (!tooling) throw Error('usage: node scripts/capture-syntax-edges.mjs UPSTREAM TOOLING')
const root = path.resolve('.cache/syntax-edges')
fs.mkdirSync(root, {recursive:true})
const source = path.resolve(upstream, 'packages')
const output = path.resolve('testdata/upstream/syntax-edges.json')
const selectors = [`a[foo[bar]]`, `a[foo[bar]baz]`, `'a\\'b'`, `a"b\\"c"`, `a\\`, `a[foo[bar`, `:is('a')`, `:is("a\\"b")`, `:where(a[foo[bar]], 'b')`, `:is(a`, `:nth-child(2n of a\\)`]
const attributes = [`[a\\:b=x]`, `[a='b\\'c']`, `[a="b\\"c"]`, `[a\\]`, `[a="b\\"]`]
const values = [`a\\`, `'a\\'b'`, `a"b\\"c"`, `var(--a\\)`, `:`, `()`, `var()`]
const types=[['linear-gradient(red,blue),var(--image)',['image']],['left var(--position)',['position']],['20px 30px',['bg-size']],['anything',['any','*','unknown']],['var(--x)',['image','position']],['var(--x),url(a)',['image']],['cover,20px',['bg-size']]]
const urls = [
 ['.a {background:url(a)}', '/a b/with"quote', '/'],
 [`.a {background:url('a.png')}`, "/can't", '/'],
 [`.a {background:url('a\"b.png')}`, '/src', '/'],
 [`.a {background:url(foo/bar(a).png)}`, '/src', '/'],
 [`.a {background:url(HTTP://example.com/a)}`, '/src', '/'],
 [`.a {background:url(Abc://a)}`, '/src', '/'],
 [`.a {background:image-set(, url(a.png) 1x, )}`, '/src', '/'],
 [`.a {background:url(a)`, '/src', '/'],
 [`.a {background:image-set("a\\\"b.png" 1x)}`, '/src', '/'],
 [`.a {background:url(a\\ b.png)}`, '/src', '/'],
 [`.a {background:url(a); mask:url(var(--mask))}`, '/src', '/'],
 [`.a {color:red; &:hover {background: url(a)}}`, '/src', '/'],
]
fs.writeFileSync(path.join(root,'edges.test.ts'), `
import fs from 'node:fs'
import {test} from 'vitest'
import * as selector from ${JSON.stringify(source+'/tailwindcss/src/selector-parser')}
import * as attribute from ${JSON.stringify(source+'/tailwindcss/src/attribute-selector-parser')}
import * as value from ${JSON.stringify(source+'/tailwindcss/src/value-parser')}
import {rewriteUrls} from ${JSON.stringify(source+'/@tailwindcss-node/src/urls')}
import {inferDataType} from ${JSON.stringify(source+'/tailwindcss/src/utils/infer-data-type')}
import {Variants} from ${JSON.stringify(source+'/tailwindcss/src/variants')}
test('capture parser edges', async()=>{
 const records=[]
 const variants=new Variants()
 variants.static('a',()=>{}, {order:100})
 variants.static('z',()=>{}, {order:100})
 for(const [a,z] of [['a','z'],['z','a']])records.push({kind:'variant-order',input:a,base:z,result:Math.sign(variants.compare({kind:'static',root:a},{kind:'static',root:z}))})
 for(const [kind,inputs,parse] of [['selector',${JSON.stringify(selectors)},selector.parse],['attribute',${JSON.stringify(attributes)},attribute.parse],['value',${JSON.stringify(values)},value.parse]]) {
  for(const input of inputs) records.push({kind,input,result:parse(input)})
 }
 for(const [input,base,root] of ${JSON.stringify(urls)}) {
  try { records.push({kind:'url',input,base,root,result:await rewriteUrls({css:input,base,root})}) }
  catch(error) { records.push({kind:'url',input,base,root,error:error.message}) }
 }
 for(const [input,types] of ${JSON.stringify(types)})records.push({kind:'type',input,types,result:inferDataType(input,types)})
 fs.writeFileSync(${JSON.stringify(output)},JSON.stringify(records,null,2)+'\\n')
})
`)
fs.writeFileSync(path.join(root,'vitest.config.mts'),`import {defineConfig} from 'vitest/config'; export default defineConfig({root:${JSON.stringify(root)},test:{include:['edges.test.ts']}})`)
const require = createRequire(path.resolve(tooling,'package.json'))
const vitest = path.join(path.dirname(require.resolve('vitest/package.json')),'vitest.mjs')
const result = spawnSync(process.execPath,[vitest,'run','--config',path.join(root,'vitest.config.mts')],{stdio:'inherit'})
process.exit(result.status??1)
