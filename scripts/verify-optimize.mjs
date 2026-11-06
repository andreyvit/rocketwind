// Check Go optimization independently with the exact optimizer used by Tailwind.
// The Go test suite itself has no Node dependency.
import fs from 'node:fs'
import path from 'node:path'
import {gunzipSync} from 'node:zlib'
import {spawnSync} from 'node:child_process'
import {createRequire} from 'node:module'
const tooling=process.argv[2]
if(!tooling)throw Error('usage: node scripts/verify-optimize.mjs TOOLING')
const require=createRequire(path.resolve(tooling,'package.json'))
const {optimize}=require('@tailwindcss/node')
const {transform}=require('lightningcss')
console.warn=()=>{} // Warnings for intentionally invalid upstream fixtures are captured separately.
const data=JSON.parse(gunzipSync(fs.readFileSync('testdata/upstream/optimize.json.gz')))
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
function clean(value){
 if(Array.isArray(value))return value.filter((item,i)=>!(item?.type==='token'&&item.value?.type==='white-space'&&(i===0||i===value.length-1||value[i-1]?.value?.type==='comma'||value[i+1]?.value?.type==='comma'))).map(clean)
 if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).filter(([key])=>key!=='loc').map(([key,item])=>[key,clean(item)]))
 return value
}
function signature(css){let result;transform({code:Buffer.from(css),errorRecovery:true,visitor:{StyleSheetExit(sheet){result=[sheet.rules,sheet.licenseComments]}}});return JSON.stringify(clean(result))}
const failures=[]
for(let i=0;i<inputs.length;i++){
 const test=data.cases[i>>1],got=outputs[i]
 if(got.error){failures.push({i,test:test.test,error:got.error});continue}
 const normalized=optimize(got.css,{minify:true}).code
 if(signature(normalized)===signature(test.compact))continue
 failures.push({i,test:test.test,minify:inputs[i].Minify,input:test.css,got:got.css,normalized,want:test.compact})
}
fs.writeFileSync(directory+'/failures.json',JSON.stringify(failures,null,2)+'\n')
console.log(`${inputs.length-failures.length}/${inputs.length} optimizer comparisons pass`)
for(const f of failures.slice(0,5))console.error(f.test,JSON.stringify(f).slice(0,600))
if(failures.length)process.exit(1)
