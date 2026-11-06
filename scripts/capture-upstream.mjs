// Run the original upstream tests and record public compiler/parser calls.
// JavaScript is development tooling only; Rocketwind itself is pure Go.
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'

const [upstream, tooling, ...tests] = process.argv.slice(2)
if (!upstream || !tooling || tests.length === 0) {
  throw new Error('usage: node scripts/capture-upstream.mjs UPSTREAM TOOLING TEST...')
}
const root = path.resolve(upstream, 'packages/tailwindcss')
const modules = path.resolve(tooling, 'node_modules')
const cache = path.resolve('.cache/capture')
const output = path.resolve('testdata/upstream')
fs.mkdirSync(cache, { recursive: true })
fs.mkdirSync(output, { recursive: true })
if (!fs.existsSync(path.join(upstream, 'node_modules'))) fs.symlinkSync(modules, path.join(upstream, 'node_modules'))
if (!fs.existsSync(path.resolve('.cache/node_modules'))) fs.symlinkSync(modules, path.resolve('.cache/node_modules'))
const config = path.join(cache, 'vitest.config.mts')
const setup = path.join(cache, 'setup.ts')
fs.writeFileSync(setup, `
import fs from 'node:fs'
import { afterAll, expect } from 'vitest'
const records = []
const candidateRegistries = new Map()
const designRegistries = new Map()
globalThis.__rocketwindSuite = () => expect.getState().testPath.replaceAll('\\\\', '/').split('/src/')[1].replace(/\\.test\\.ts$/, '')
globalThis.__rocketwindRecord = (kind, args, result, error) => {
  const testPath = expect.getState().testPath
  if (kind === 'css.parse' && !testPath.endsWith('/css-parser.test.ts')) return
  if (kind === 'value.decode' && !testPath.endsWith('/decode-arbitrary-value.test.ts')) return
  if (kind.startsWith('internal:') && !testPath.endsWith('/' + kind.split(':')[1] + '.test.ts')) return
  if (kind.startsWith('internal:candidate:')) {
    const key = JSON.stringify(args[1])
    const found = candidateRegistries.get(key)
    if (found !== undefined) args[1] = {$registry:found}
    else {const id = candidateRegistries.size;candidateRegistries.set(key,id);args[1] = {$registry:id,definition:args[1]}}
  }
  if ((kind === 'design.canonicalize' || kind === 'design.order') && testPath.endsWith('/canonicalize-candidates.test.ts')) {
    const definition = {css:args[0],options:args[1]}
    const key = JSON.stringify(definition)
    const found = designRegistries.get(key)
    let ref
    if (found !== undefined) ref = {$design:found}
    else {const id = designRegistries.size;designRegistries.set(key,id);ref = {$design:id,definition}}
    args.splice(0,2,ref)
  }
  records.push(JSON.parse(JSON.stringify({ test: expect.getState().currentTestName, kind, args, result, error }, (key,value)=>typeof value === 'bigint' ? Number(value) : typeof value === 'function' ? { $function:value.toString() } : value)))
}
afterAll(() => {
  const name = expect.getState().testPath.replaceAll('\\\\', '/').split('/src/')[1].replaceAll('/', '_')
  fs.writeFileSync(${JSON.stringify(output)} + '/' + name + '.json', JSON.stringify(records, null, 2) + '\\n')
})
`)
fs.writeFileSync(config, `
import { defineConfig } from 'vitest/config'
export default defineConfig({
  root: ${JSON.stringify(root)},
  plugins: [{
    name: 'rocketwind-record', enforce: 'pre',
    transform(code, id) {
      const pure = {
        'value-parser': ['parse', 'toCss'],
        'selector-parser': ['parse', 'toCss'],
        'attribute-selector-parser': ['parse'],
        'utils/segment': ['segment'],
        'utils/escape': ['escape', 'unescape'],
        'utils/brace-expansion': ['expand'],
        'utils/compare': ['compare'],
        'utils/to-key-path': ['toKeyPath'],
        'utils/replace-shadow-colors': ['replaceShadowColors'],
        'source-maps/visualize-source-map': ['visualizeSourceMap', 'visualizeSourceMapRanges'],
        'canonicalize-calc-expressions': ['canonicalizeCalcExpressions'],
        'constant-fold-declaration': ['constantFoldDeclaration'],
        'expand-declaration': ['expandDeclaration'],
      }
      for (const [file, names] of Object.entries(pure)) {
        if (!id.endsWith('/src/' + file + '.ts')) continue
        for (const name of names) {
          code = code.replace('export function ' + name + '(', 'function original_' + name + '(')
          code += \`
export function \${name}(...args) {
            try {
              const result = original_\${name}(...args)
              globalThis.__rocketwindRecord?.('internal:\${file}:\${name}', args, result, undefined)
              return result
            } catch (error) {
              globalThis.__rocketwindRecord?.('internal:\${file}:\${name}', args, undefined, error.message)
              throw error
            }
          }\`
        }
        return code
      }
      if (id.endsWith('/src/source-maps/line-table.ts')) {
        code = code.replace('export function createLineTable(', 'function originalCreateLineTable(')
        return code + \`
          export function createLineTable(source) {
            const table = originalCreateLineTable(source)
            return Object.fromEntries(Object.entries(table).map(([name,fn])=>[name,(...args)=>{
              const result = fn(...args)
              globalThis.__rocketwindRecord?.('internal:source-maps/line-table:'+name,[source,...args],result,undefined)
              return result
            }]))
          }
        \`
      }
      if (id.endsWith('/src/canonicalize-candidates.ts')) {
        code = code.replace('export function intersection<T>(', 'function originalIntersection<T>(')
        return code + \`
          export function intersection(a,b) {
            const result = originalIntersection(a,b)
            globalThis.__rocketwindRecord?.('internal:canonicalize-candidates:intersection',[[...a],[...b]],[...result],undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/src/source-maps/source-map.ts')) {
        code = code.replace('export function createTranslationMap(', 'function originalCreateTranslationMap(')
        return code + \`
          export function createTranslationMap(options) {
            const translate = originalCreateTranslationMap(options)
            return (node) => {
              const result = translate(node)
              globalThis.__rocketwindRecord?.('internal:source-maps/translation-map:translate',[options,node],result,undefined)
              return result
            }
          }
        \`
      }
      if (id.endsWith('/src/compat/config/create-compat-config.ts')) {
        code = code.replace('export function createCompatConfig(', 'function originalCreateCompatConfig(')
        return code + \`
          export function createCompatConfig(...args) {
            const result = originalCreateCompatConfig(...args)
            Object.defineProperty(result, '__rocketwindCompat', {value:true})
            return result
          }
        \`
      }
      if (id.endsWith('/src/compat/config/resolve-config.ts')) {
        code = code.replace('export function resolveConfig(', 'function originalResolveConfig(')
        return code + \`
          export function resolveConfig(design, files) {
            const input = files.map(file=>({...file,config:file.config.__rocketwindCompat ? {$compat:true} : file.config}))
            const result = originalResolveConfig(design,files)
            const suite = globalThis.__rocketwindSuite?.()
            if (suite === 'compat/config/resolve-config' || suite === 'compat/config/create-compat-config') {
              globalThis.__rocketwindRecord?.('internal:'+suite+':resolve', [input,{theme:[...design.theme.values],prefix:design.theme.prefix}], {config:result.resolvedConfig,replaced:[...result.replacedThemeKeys]}, undefined)
            }
            return result
          }
        \`
      }
      if (id.endsWith('/src/compat/apply-config-to-theme.ts')) {
        code = code.replace('export function applyConfigToTheme(', 'function originalApplyConfigToTheme(')
        code = code.replace('export function keyPathToCssProperty(', 'function originalKeyPathToCssProperty(')
        return code + \`
          export function applyConfigToTheme(design,config,replaced) {
            const before = [...design.theme.values].map(([key,value])=>[key,{...value}])
            const result = originalApplyConfigToTheme(design,config,replaced)
            globalThis.__rocketwindRecord?.('internal:compat/apply-config-to-theme:apply',[before,config,[...replaced]],[...design.theme.values],undefined)
            return result
          }
          export function keyPathToCssProperty(path) {
            const result = originalKeyPathToCssProperty(path)
            globalThis.__rocketwindRecord?.('internal:compat/apply-config-to-theme:key',[path],result,undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/src/compat/apply-keyframes-to-theme.ts')) {
        code = code.replace('export function applyKeyframesToTheme(', 'function originalApplyKeyframesToTheme(')
        return code + \`
          export function applyKeyframesToTheme(design,config) {
            const before = JSON.parse(JSON.stringify(design.theme.getKeyframes()))
            const result = originalApplyKeyframesToTheme(design,config)
            globalThis.__rocketwindRecord?.('internal:compat/apply-keyframes-to-theme:apply',[before,config],design.theme.getKeyframes(),undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/src/compat/flatten-color-palette.ts')) {
        code = code.replace('export default function flattenColorPalette(', 'function originalFlattenColorPalette(')
        return code + \`
          export default function flattenColorPalette(colors) {
            const result = originalFlattenColorPalette(colors)
            globalThis.__rocketwindRecord?.('internal:compat/flatten-color-palette:flatten',[colors],result,undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/src/ast.ts')) {
        code = code.replace('export function optimizeAst(', 'function originalOptimizeAst(')
        code = code.replace('export function handleNesting(', 'function originalHandleNesting(')
        code = code.replace('export function toCss(', 'function originalAstToCss(')
        return code + \`
          export function handleNesting(ast) {
            const input = JSON.parse(JSON.stringify(ast))
            const result = originalHandleNesting(ast)
            globalThis.__rocketwindRecord?.('internal:ast:nesting',[input],originalAstToCss(result),undefined)
            return result
          }
          export function optimizeAst(ast, design, polyfills) {
            const input = JSON.parse(JSON.stringify(ast))
            const result = originalOptimizeAst(ast,design,polyfills)
            globalThis.__rocketwindRecord?.('internal:ast:optimize',[input,{prefix:design.theme.prefix,theme:[...design.theme.values]},polyfills],originalAstToCss(result),undefined)
            return result
          }
          export function toCss(...args) {
            const input = JSON.parse(JSON.stringify(args))
            const result = originalAstToCss(...args)
            globalThis.__rocketwindRecord?.('internal:ast:print',input,result,undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/src/candidate.ts')) {
        code = code.replace('export function* parseCandidate(', 'function* originalParseCandidate(')
        code = code.replace('export function printCandidate(', 'function originalPrintCandidate(')
        return code + \`
          function candidateMetadata(design) {
            return {prefix:design.theme.prefix, static:design.utilities.keys('static'), functional:design.utilities.keys('functional'), variants:[...design.variants.variants].map(([root,v])=>({root,kind:v.kind,order:v.order,compounds:v.compounds,compoundsWith:v.compoundsWith})), theme:[...design.theme.values]}
          }
          export function* parseCandidate(input, design) {
            const result = [...originalParseCandidate(input,design)]
            globalThis.__rocketwindRecord?.('internal:candidate:parse', [input,candidateMetadata(design)], result, undefined)
            yield* result
          }
          export function printCandidate(design,candidate) {
            const result = originalPrintCandidate(design,candidate)
            globalThis.__rocketwindRecord?.('internal:candidate:print', [candidate,candidateMetadata(design)], result, undefined)
            return result
          }
        \`
      }
      if (id.endsWith('/tailwindcss/src/index.ts')) {
        code = code.replace('export async function compile(', 'async function compileOriginal(')
        code = code.replace('export async function __unstable__loadDesignSystem(', 'async function loadDesignOriginal(')
        return code + \`
          function recordLoader(original, records) {
            if (!original) return original
            return async (...args) => {
              try {
                const result = await original(...args)
                const record = { args, result: JSON.parse(JSON.stringify(result, (key, value) => typeof value === 'function' ? { $function: value.toString() } : value)) }
                records.push(record)
                if (args[2] === 'plugin') return { ...result, module: capturePlugin(result.module, record) }
                if (args[2] === 'config' && result.module?.plugins) return { ...result, module: { ...result.module, plugins: result.module.plugins.map(p => capturePlugin(p, record)) } }
                return result
              } catch (error) {
                records.push({ args, error: error.message })
                throw error
              }
            }
          }
          function capturePlugin(plugin, record) {
            if (plugin.__isOptionsFunction) {
              const wrapper = (options) => capturePlugin(plugin(options), record)
              Object.assign(wrapper, plugin)
              return wrapper
            }
            const handler = typeof plugin === 'function' ? plugin : plugin.handler
            if (typeof handler !== 'function') return plugin
            return { ...(typeof plugin === 'object' ? plugin : {}), handler(api) {
              const operations = []
              ;(record.plugins ??= []).push(operations)
              return handler(new Proxy(api, { get(target, key) {
                const original = target[key]
                if (typeof original !== 'function') return original
                return (...args) => {
                  const clone = value => JSON.parse(JSON.stringify(value, (key, value) => typeof value === 'function' ? {$function:value.toString()} : value === undefined ? {$undefined:true} : value))
                  const op = {name:key, args:clone(args), calls:[]}
                  operations.push(op)
                  const capture = (name, fn) => (...values) => {
                    try {
                      const result = fn(...values)
                      op.calls.push({name,args:clone(values),result:clone(result)})
                      return result
                    } catch (error) {
                      op.calls.push({name,args:clone(values),error:error.message})
                      throw error
                    }
                  }
                  if (key === 'matchUtilities' || key === 'matchComponents') {
                    args[0] = Object.fromEntries(Object.entries(args[0]).map(([name,fn])=>[name,capture(name,fn)]))
                    if (typeof args[1]?.values?.__BARE_VALUE__ === 'function') args[1] = {...args[1],values:{...args[1].values,__BARE_VALUE__:capture('__BARE_VALUE__',args[1].values.__BARE_VALUE__)}}
                  } else if (key === 'matchVariant') {
                    args[1] = capture('variant',args[1])
                    if (typeof args[2]?.sort === 'function') {
                      const sort = args[2].sort
                      const values = new Map()
                      const seen = new Set()
                      args[2] = {...args[2],sort(a,b) {
                        values.set(JSON.stringify(a),a); values.set(JSON.stringify(b),b)
                        for (const x of values.values()) for (const y of values.values()) {
                          const key = JSON.stringify([x,y])
                          if (seen.has(key)) continue
                          seen.add(key)
                          ;(op.sortCases ??= []).push({args:clone([x,y]),result:sort(x,y)})
                        }
                        return sort(a,b)
                      }}
                    }
                  }
                  const result = original(...args)
                  op.result = clone(result)
                  return result
                }
              }}))
            }}
          }
          export async function __unstable__loadDesignSystem(css,options = {}) {
            const metadata = JSON.parse(JSON.stringify(options))
            const recordedOptions = {...options}
            if (options.loadStylesheet) recordedOptions.loadStylesheet = recordLoader(options.loadStylesheet, metadata.stylesheets = [])
            if (options.loadModule) recordedOptions.loadModule = recordLoader(options.loadModule, metadata.modules = [])
            const design = await loadDesignOriginal(css,recordedOptions)
            if (globalThis.__rocketwindSuite?.() === 'canonicalize-candidates') {
              const original = design.canonicalizeCandidates.bind(design)
              design.canonicalizeCandidates = (...args) => {
                const result = original(...args)
                globalThis.__rocketwindRecord?.('design.canonicalize',[css,metadata,...args],result,undefined)
                return result
              }
            }
            if (globalThis.__rocketwindSuite?.() === 'intellisense') {
              const state = () => ({theme:[...design.theme.values],invalid:[...design.invalidCandidates]})
              for (const name of ['getClassList','candidatesToCss','candidatesToAst']) {
                const original = design[name].bind(design)
                design[name] = (...args) => {
                  const before = state()
                  const result = original(...args)
                  globalThis.__rocketwindRecord?.('design.'+name,[css,metadata,before,...args],result,undefined)
                  return result
                }
              }
              const original = design.getVariants.bind(design)
              design.getVariants = () => {
                const result = original()
                globalThis.__rocketwindRecord?.('design.getVariants',[css,metadata,state()],result.map(({selectors,...rest})=>rest),undefined)
                return result.map(({selectors,...rest})=>({...rest,selectors:(...args)=>{
                  const result = selectors(...args)
                  globalThis.__rocketwindRecord?.('design.selectors',[css,metadata,state(),rest.name,...args],result,undefined)
                  return result
                }}))
              }
            }
            const getClassOrder = design.getClassOrder.bind(design)
            design.getClassOrder = (classes) => {
              const result = getClassOrder(classes)
              globalThis.__rocketwindRecord?.('design.order',[css,metadata,classes],result,undefined)
              return result
            }
            return design
          }
          export async function compile(css, options = {}) {
            const metadata = JSON.parse(JSON.stringify(options))
            const recordedOptions = { ...options }
            if (options.loadStylesheet) recordedOptions.loadStylesheet = recordLoader(options.loadStylesheet, metadata.stylesheets = [])
            if (options.loadModule) recordedOptions.loadModule = recordLoader(options.loadModule, metadata.modules = [])
            try {
              const api = await compileOriginal(css, recordedOptions)
              globalThis.__rocketwindRecord?.('compile.metadata', [css, metadata], JSON.stringify({sources:api.sources,root:api.root,features:api.features}), undefined)
              const builds = []
              return {...api, build(candidates) {
                builds.push([...candidates])
                try {
                  const result = api.build(candidates)
                  globalThis.__rocketwindRecord?.('compile.build', [css, metadata, builds.slice()], result, undefined)
                  return result
                } catch (error) {
                  globalThis.__rocketwindRecord?.('compile.build', [css, metadata, builds.slice()], undefined, error.message)
                  throw error
                }
              }, buildSourceMap() {
                const result = api.buildSourceMap()
                globalThis.__rocketwindRecord?.('compile.map', [css, metadata, builds.slice()], result, undefined)
                return result
              }}
            } catch (error) {
              globalThis.__rocketwindRecord?.('compile', [css, metadata], undefined, error.message)
              throw error
            }
          }
        \`
      }
      if (id.endsWith('/src/utils/decode-arbitrary-value.ts')) {
        code = code.replace('export function decodeArbitraryValue(', 'function decodeOriginal(')
        return code + \`
          export function decodeArbitraryValue(...args) {
            const result = decodeOriginal(...args)
            globalThis.__rocketwindRecord?.('value.decode', args, result, undefined)
            return result
          }
        \`
      }
      if (!id.endsWith('/src/css-parser.ts')) return
      code = code.replace('export function parse(', 'function parseOriginal(')
      return code + \`
        export function parse(...args) {
          try {
            const result = parseOriginal(...args)
            globalThis.__rocketwindRecord?.('css.parse', args, result, undefined)
            return result
          } catch (error) {
            globalThis.__rocketwindRecord?.('css.parse', args, undefined, error.message)
            throw error
          }
        }
      \`
    }
  }],
  test: {
    setupFiles: [${JSON.stringify(setup)}],
    snapshotSerializers: [${JSON.stringify(path.join(root, 'src/test-utils/custom-serializer.ts'))}],
    maxWorkers: 2,
  }
})
`)
const require = createRequire(path.join(tooling, 'package.json'))
const cli = path.join(path.dirname(require.resolve('vitest/package.json')), 'vitest.mjs')
const result = spawnSync(process.execPath, [cli, 'run', '--config', config, ...tests], { stdio: 'inherit' })
process.exit(result.status ?? 1)
