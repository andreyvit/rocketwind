import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
const [tooling] = process.argv.slice(2)
if (!tooling) throw Error('usage: node scripts/capture-compiler-edges.mjs TOOLING')
const require = createRequire(path.resolve(tooling,'package.json'))
const {compile} = require('tailwindcss')
const cases = [
 {css:'@media screen {@import "card";}',sheets:{card:'.card {color:red}'}},
 {css:'@import "missing";'},
 {css:'@import "missing";',loadError:'stylesheet unavailable'},
 {css:'@import "bad";',sheets:{bad:'.a{'}},
 {css:'@media screen {@import "missing";}',loadError:'stylesheet unavailable'},
 ...['file','layer(x)','"card" layer(a) layer(b)','"card" supports(display:grid) supports(display:flex)'].map(value=>({css:'@import '+value+';',sheets:{card:'.card {color:red}'}})),
 ...['@source "a" {color:red}', '.a {@source "a";}', '@source a;', '@source inline("{a,{1..3..0}}");'].map(css=>({css})),
 {css:'@media prefix(tw) {@theme prefix(no) {--color-a:red}}'},
 {css:'@media reference {@source a;}'},
 {css:'@theme {--color-*:red;}'},
 {css:'@theme {--color-brand:theme(colors.missing);}'},
 {css:'@import "a" theme(reference);',sheets:{a:'@media screen {.a{color:red}}'}},
 ...['bad%name','bad.name','bad..name','bad.','bad.letter','bad-%'].map(name=>({css:'@utility '+name+' {color:red;}'})),
 {css:'@utility demo-* {&:hover {width:--value(integer);}} @tailwind utilities;',classes:['demo-red']},
 {css:'@utility demo-* {width:--value(--size-*-*,--missing-*,[length]);} @tailwind utilities;',classes:['demo-[3px]','demo-4']},
 {css:'@utility demo-* {width:--value(ratio); &:hover {height:--modifier(integer);}} @tailwind utilities;',classes:['demo-3/4']},
 {css:'@custom-variant x ();'},
 {css:'@variant min-missing { .a { color:red; }}'},
 {css:'@utility demo-* {@variant min-missing {width:--value(integer);}} @tailwind utilities;',classes:['demo-1']},
 {css:'@utility demo {color:blue;} @utility demo {@variant min-missing {color:red;}} @tailwind utilities;',classes:['demo']},
 {css:'@theme {--color-a:var(--color-b);--color-b:var(--color-a);} @tailwind utilities;',classes:['bg-a/50']},
 {css:'@theme {--color-a:var(--color-b);--color-b:currentColor;} @tailwind utilities;',classes:['bg-a/50']},
 {css:'@theme {--color-a:var(--color-b);--color-b:var();} @tailwind utilities;',classes:['bg-a/50']},
 {css:'@theme {--background-image-pattern:linear-gradient(red,blue);} @tailwind utilities;',classes:['bg-pattern']},
 {css:'@theme {--text-brand:1rem;--text-brand--letter-spacing:.1em;} @tailwind utilities;',classes:['text-brand']},
 {css:'@tailwind utilities;',classes:['stroke-[length:var(--width)]','stroke-[number:2]','stroke-[3px]']},
 {css:'@source inline("flex text-xl"); @tailwind utilities;'},
 ...[
  '.a{@apply flex !important;}', '.a{@apply !flex;}',
  '.a{@apply missing:flex;}', '@tailwind utilities;.a{@apply missing;}',
  '@utility same {color:blue;} @utility same {@apply missing;}',
  '@utility demo-* {width:--value(integer);@apply missing;}',
  '@utility demo-* {@apply demo-1;} .a{@apply demo-1;}',
  '@utility demo-* {@apply other-1;} @utility other-* {@apply demo-1;} .a{@apply demo-1;}',
  '@utility demo-* {@apply flex;} .a{@apply demo-1;}',
 ].map(css=>({css})),
 {css:'@theme {--color-blank:;} @tailwind utilities;',classes:['text-blank','bg-blank']},
 {css:'@theme inline {--color-blank:;} @tailwind utilities;',classes:['bg-blank']},
 {css:'@theme {--color-blank:;} @theme default{--color-blank:red;} @tailwind utilities;',classes:['bg-blank']},
 {css:'@theme reference {--color-blank:;} @tailwind utilities;',classes:['bg-blank']},
 {css:'@tailwind utilities;',classes:['supports-[not_(display:grid)]:flex','supports-[selector_(a)]:flex','supports-[font-format_(opentype)]:flex','group-min-missing:flex','not-before:flex','not-has-before:flex','has-hover/foo:flex']},
 {css:'@custom-variant mixed {&:hover {color:red;@slot;}} @tailwind utilities;',classes:['not-mixed:flex']},
 {css:'@custom-variant pseudo (&::before);@tailwind utilities;',classes:['not-pseudo:flex']},
 ...[
  {darkMode:['variant',[]]}, {darkMode:['variant','.dark']}, {darkMode:['variant','[data-dark]']},
  {content:[{raw:'<div class="flex">'}]}, {content:[{base:'/templates',pattern:'*.html'}]},
  {theme:{screens:{blank:[null,false]}}},
 ].map(config=>({css:'@config "config";@tailwind utilities;',config,classes:['dark:flex']})),
]
for (const c of cases) {
 const options = {}
 if(c.config)options.loadModule=async()=>({base:'',module:c.config})
 if (c.sheets || c.loadError) options.loadStylesheet = async id => {
  if(c.loadError)throw Error(c.loadError)
  if (!(id in c.sheets))throw Error('missing fixture stylesheet: '+id)
  return {base:'',path:id,content:c.sheets[id]}
 }
 try { const result=await compile(c.css, options); c.result=result.build(c.classes??[]) }
 catch (error) { c.error=error.message }
}
fs.writeFileSync('testdata/upstream/compiler-edges.json',JSON.stringify(cases,null,2)+'\n')
console.log(`Captured ${cases.length} compiler edge cases`)
