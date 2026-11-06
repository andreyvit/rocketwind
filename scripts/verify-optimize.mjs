// Check Go optimization independently with the exact optimizer used by Tailwind.
// The Go test suite itself has no Node dependency.
import fs from 'node:fs'
import path from 'node:path'
import {gunzipSync,gzipSync} from 'node:zlib'
import {createHash} from 'node:crypto'
import {spawnSync} from 'node:child_process'
import {createRequire} from 'node:module'
const tooling=process.argv[2]
if(!tooling)throw Error('usage: node scripts/verify-optimize.mjs TOOLING')
const require=createRequire(path.resolve(tooling,'package.json'))
const {transform}=require('lightningcss')
console.warn=()=>{} // Warnings for intentionally invalid upstream fixtures are captured separately.
const data=JSON.parse(gunzipSync(fs.readFileSync('testdata/upstream/optimize.json.gz')))
if(fs.existsSync('testdata/upstream/browser-edges.json'))data.cases.push(...JSON.parse(fs.readFileSync('testdata/upstream/browser-edges.json')).cases)
const directory='.cache/verify-optimize'
fs.mkdirSync(directory,{recursive:true})
fs.writeFileSync(directory+'/main.go',`package main
import("encoding/json";"os";"github.com/andreyvit/rocketwind")
func main(){var inputs []struct{CSS string;Minify bool};if err:=json.NewDecoder(os.Stdin).Decode(&inputs);err!=nil{panic(err)};result:=make([]any,len(inputs));for i,input:=range inputs{out,err:=rocketwind.Optimize(input.CSS,rocketwind.OptimizeOptions{Minify:input.Minify});if err!=nil{result[i]=map[string]string{"error":err.Error()}}else{result[i]=map[string]string{"css":out}}};if err:=json.NewEncoder(os.Stdout).Encode(result);err!=nil{panic(err)}}
`)
const build=spawnSync('go',['build','-o',directory+'/probe','./'+directory],{stdio:'inherit'})
if(build.status!==0)process.exit(build.status??1)
const inputs=data.cases.flatMap(test=>[{CSS:test.css,Minify:false},{CSS:test.css,Minify:true}])
const run=spawnSync(directory+'/probe',[],{input:JSON.stringify(inputs),encoding:'utf8',maxBuffer:100*1024*1024})
if(run.status!==0)throw Error(run.stderr)
const outputs=JSON.parse(run.stdout)
function clean(value,key){
 if(Array.isArray(value)){
  let result=value.filter((item,i)=>!(item?.type==='token'&&item.value?.type==='white-space'&&(i===0||i===value.length-1||value[i-1]?.value?.type==='comma'||value[i+1]?.value?.type==='comma'))).map(item=>clean(item))
  // A selector list is equivalent to adjacent rules with those selectors.
  // Preserve rule order; only expand the grouping in each individual rule.
  result=result.flatMap(item=>item?.type==='style'?item.value.selectors.map(selector=>({...item,value:{...item.value,selectors:[selector]}})):[item])
  // Tailwind invokes Lightning CSS twice, which can duplicate the same
  // prefixed unparsed declaration. Only identical adjacent declarations
  // are redundant: different fallback values and their order still matter.
  if(key==='declarations'||key==='importantDeclarations'){
   // Custom-property definitions commute with ordinary declarations; moving
   // them does not change substitution or the cascade. Keep the order within
   // each partition, including repeated definitions and ordinary fallbacks.
   const custom=item=>item.property==='custom'&&item.value.name.startsWith('--')
   result=[...result.filter(item=>!custom(item)),...result.filter(custom)]
   // Legacy WebKit mask shorthands do not contain these two properties.
   // Lightning repeats them while combining standard and legacy masks;
   // identical repetitions have no effect, even across the shorthand.
   const maskValues=new Map()
   result=result.filter(item=>{
    const name=item.property==='custom'?item.value.name:item.property
    if(name!=='-webkit-mask-composite'&&name!=='-webkit-mask-source-type')return true
    const value=JSON.stringify(item);if(maskValues.get(name)===value)return false
    maskValues.set(name,value);return true
   })
   result=result.filter((item,i)=>i===0||JSON.stringify(item)!==JSON.stringify(result[i-1]))
  }
  return result
 }
 if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).filter(([key])=>key!=='loc').map(([key,item])=>[key,clean(item,key)]))
 return value
}
function signature(css){let result;transform({code:Buffer.from(css),errorRecovery:true,visitor:{StyleSheetExit(sheet){result=[sheet.rules,sheet.licenseComments]}}});return JSON.stringify(clean(result))}
function normalize(css){let code=Buffer.from(css);for(let i=0;i<2;i++)code=transform({code,minify:true,errorRecovery:true}).code;return code.toString()}
function firstDifference(a,b,path=''){
 if(JSON.stringify(a)===JSON.stringify(b))return
 if(a&&b&&typeof a==='object'&&typeof b==='object'){
  for(const key of new Set([...Object.keys(a),...Object.keys(b)])){
   const diff=firstDifference(a[key],b[key],path+'/'+key);if(diff)return diff
  }
 }
 return {path,got:a,want:b}
}
const failures=[]
for(let i=0;i<inputs.length;i++){
 const test=data.cases[i>>1],got=outputs[i]
 if(got.error||test.error){if(got.error!==test.error)failures.push({i,test:test.test,error:got.error,want:test.error});continue}
 // Normalize spelling and shorthands without browser targets. Using Tailwind's
 // optimizer here would silently add vendor prefixes missing from Go output.
 const normalized=normalize(got.css)
 const actual=signature(normalized),expected=signature(normalize(test.compact))
 if(actual===expected)continue
 failures.push({i,test:test.test,minify:inputs[i].Minify,difference:firstDifference(JSON.parse(actual),JSON.parse(expected)),input:test.css,got:got.css,normalized,want:test.compact})
}
fs.writeFileSync(directory+'/failures.json',JSON.stringify(failures,null,2)+'\n')
console.log(`${inputs.length-failures.length}/${inputs.length} optimizer comparisons pass`)
for(const f of failures.slice(0,5))console.error(f.test,JSON.stringify(f).slice(0,600))
if(failures.length)process.exit(1)
if(process.argv.includes('--update')){
 const cases=data.cases.map((test,i)=>({inputHash:createHash('sha256').update(test.css).digest('hex'),pretty:outputs[i*2].css,compact:outputs[i*2+1].css,error:outputs[i*2].error}))
 fs.writeFileSync('testdata/upstream/optimize-go.json.gz',gzipSync(JSON.stringify({tailwind:data.tailwind,lightningcss:data.lightningcss,cases})+'\n'))
 console.log('Saved Go outputs after all independent comparisons passed')
}
