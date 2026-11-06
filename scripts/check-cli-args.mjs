// Differential CLI argument cases beyond the original suite. Node 26+ strips TypeScript.
import fs from 'node:fs'
import path from 'node:path'
import {pathToFileURL} from 'node:url'
const upstream=process.argv[2]
if(!upstream)throw new Error('usage: node scripts/check-cli-args.mjs UPSTREAM')
const {args}=await import(pathToFileURL(path.resolve(upstream,'packages/@tailwindcss-cli/src/utils/args.ts')))
const result=[]
const types=['boolean','number','string','boolean | string','number | string','boolean | number','boolean | number | string']
const values=['','-','true','false','0','1','0x10000000000000000','1e309','-1e309','1e-400','0xfffffffffffffffffffffffff','NaN','+Inf','Infinity',' 1 ','\ufeff1\ufeff',' \t','-1','1e3','01','0x1p2','1_000','.5','0x','0b2','0o8','--','--no-v','x','𝕩','0xfffffffffffffffffffffffffg','\u00851\u0085','\u20031\ufeff','1\u0085']
for(const type of types) for(const value of values) for(const mode of ['=',' ','alias']){
 const options={'--value':{type,alias:'-v'}}
 const argv=mode==='='?['--value='+value]:[mode==='alias'?'-v':'--value',value]
 result.push({type,argv:[...argv],result:args(options,[...argv])})
}
for(const argv of [['-abc=7'],['---value=x'],['--no-value'],['--no-value=false'],['--=x'],['---'],['-'],['--'],['--value='],[''],['-vv'],['--value','1','-v','2'],['-v','1','--value','2'],['--','--value','-']])for(const type of types){result.push({type,argv,result:args({'--value':{type,alias:'-v'}},[...argv])})}
fs.writeFileSync('testdata/upstream/cli-args-extra.json',JSON.stringify(result,(key,value)=>typeof value==='number'&&!Number.isFinite(value)?{$number:String(value)}:value,2)+'\n')
console.log(result.length)
