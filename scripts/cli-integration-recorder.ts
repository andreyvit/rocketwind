import nodeFS from 'node:fs'
import path from 'node:path'
import {gzipSync} from 'node:zlib'
import {createHash} from 'node:crypto'

export function recordIntegration(context: any, name: string, file: string) {
  const actions: any[]=[]
  const root=context.root
  let checking=0
  const record=(value: any)=>actions.push(value)
  function data(value: any): any {
    if(typeof value==='string')return value
      .replace(/sourceMappingURL=data:application\/json;base64,([A-Za-z0-9+/=]+)/g, (_, encoded) =>
        'sourceMappingURL=data:application/json;base64,' + Buffer.from(data(Buffer.from(encoded,'base64').toString())).toString('base64'))
      .replaceAll('/private'+root,'__ROOT__')
      .replaceAll(root,'__ROOT__')
      .replace(/(?:\/[^\s"'\\]+)*\/tailwindcss\/packages\/@tailwindcss-standalone\/dist\//g,'__UPSTREAM__/standalone/')
      .replace(/file:(?:\/[^\s"'\\]+)*\/tailwindcss-4\.3\.4\.tgz/g,'file:__REFERENCE__/tailwindcss-4.3.4.tgz')
      .replace(/Done in [0-9.]+(?:ns|µs|ms|s)/g,'Done in <duration>')
    if(value instanceof RegExp)return {$regexp:value.source,flags:value.flags}
    if(value instanceof Uint8Array)return {$base64:Buffer.from(value).toString('base64')}
    if(Array.isArray(value))return value.map(data)
    if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([k,v])=>[k,data(v)]))
    return value
  }
  for(const method of ['write','delete','symlink','create']){
    const original=context.fs[method].bind(context.fs)
    context.fs[method]=async(...args: any[])=>{const result=await original(...args);record({kind:method,args:data(args)});return result}
  }
  for(const method of ['expectFileToContain','expectFileNotToContain']){
    const original=context.fs[method].bind(context.fs)
    context.fs[method]=async(...args: any[])=>{checking++;try{const result=await original(...args);record({kind:method,args:data(args)});return result}finally{checking--}}
  }
  for(const method of ['read','glob']){
    const original=context.fs[method].bind(context.fs)
    context.fs[method]=async(...args: any[])=>{const result=await original(...args);if(!checking)record({kind:method,args:data(args),result:data(result)});return result}
  }
  const originalExec=context.exec.bind(context)
  context.exec=async(...args:any[])=>{
    const entry={kind:'exec',args:data(args)} as any
    record(entry)
    try{const result=await originalExec(...args);if(args[0]!=='pnpm install')entry.result=data(result);return result}
    catch(error:any){entry.error=data(String(error));entry.stderr=data(error.stderr);throw error}
  }
  const originalSpawn=context.spawn.bind(context)
  context.spawn=async(...args:any[])=>{
    record({kind:'spawn',args:data(args)})
    const process=await originalSpawn(...args)
    for(const method of ['onStderr','onStdout']){
      const original=process[method].bind(process)
      process[method]=async(predicate:any)=>{
        let matched=''
        const result=await original((message:string)=>{if(predicate(message)){matched=message;return true}return false})
        record({kind:method,message:data(matched)})
        return result
      }
    }
    return process
  }
  const originalMap=context.parseSourceMap
  context.parseSourceMap=(...args:any[])=>{
    const sourceMap=originalMap(...args)
    return {...sourceMap,at(line:number,column:number){const result=sourceMap.at(line,column);record({kind:'map',args:data(args),line,column,result:data(result)});return result}}
  }
  return {finish(){
    const key=path.basename(file)+'-'+createHash('sha256').update(name).digest('hex').slice(0,12)
    nodeFS.writeFileSync(path.join(__OUTPUT_DIRECTORY__,key+'.json.gz'),gzipSync(JSON.stringify({file:path.basename(file),name,actions})+'\n'))
  }}
}
