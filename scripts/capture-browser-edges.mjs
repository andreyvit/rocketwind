// Browser behavior beyond the CSS emitted by Tailwind's core test suite.
// Capture with the exact optimizer shipped in Tailwind 4.3.4.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
const tooling=process.argv[2]
if(!tooling)throw Error('usage: node scripts/capture-browser-edges.mjs TOOLING')
const require=createRequire(path.resolve(tooling,'package.json'))
const {optimize}=require('@tailwindcss/node')
const cases=[]
function add(css){
 const entry={test:'browser edges: '+css,css}
 try{entry.pretty=optimize(css,{minify:false}).code;entry.compact=optimize(css,{minify:true}).code}catch(error){entry.error=error.message}
 cases.push(entry)
}
const properties={
 'transition-property':['backdrop-filter','transform, filter, backdrop-filter','mask-image, user-select, color','-webkit-backdrop-filter, backdrop-filter'],

 'backdrop-filter':['blur(1px)','none'], 'user-select':['none','text'],
 'hyphens':['auto','none'], 'box-decoration-break':['clone','slice'],
 'print-color-adjust':['exact','economy'], 'text-size-adjust':['100%','none'],
 'text-decoration-skip-ink':['auto','none'],
 'text-decoration':['underline','underline dotted','underline red','underline 12%','underline solid currentColor','underline 2px','underline calc(1px + 1em)'],
 'text-decoration-color':['red','currentColor','color-mix(in oklab, red, blue)','color-mix(in oklab, currentColor, blue)'],
 'text-decoration-style':['wavy'], 'text-decoration-line':['underline'],
 'text-decoration-thickness':['12%','0%','-0%','0.1234567%','calc(10% + 2px)'],
 'background-clip':['border-box','text','padding-box, text'],
 'mask-image':['none','url(a)','linear-gradient(red, blue)'],
 'mask-position':['center','0 0'], 'mask-position-x':['left'], 'mask-position-y':['top'],
 'mask-size':['cover','20px 30px'], 'mask-repeat':['no-repeat','repeat-x'],
 'mask-clip':['border-box','no-clip'], 'mask-origin':['content-box','fill-box'],
 'mask-composite':['add','subtract','intersect','exclude','add, intersect'],
 'mask-mode':['alpha','luminance','match-source','alpha, match-source'],
 'mask':['url(a)','url(a) luminance subtract','linear-gradient(red,blue) center / cover no-repeat add alpha','url(a) subtract, url(b) exclude alpha','url(a) intersect match-source'],
 'mask-border':['url(a) 20 / 2 stretch luminance'],
 'mask-border-source':['url(a)'], 'mask-border-slice':['20 fill'],
 'mask-border-width':['2'], 'mask-border-outset':['1'], 'mask-border-repeat':['round'],
 'width':['stretch','fill-available','min-content','max-content','fit-content'],
 'background-image':['image-set("a.png" 1x, "b.png" 2x)','image-set(url(a) 1x, url(b) 2x)','cross-fade(url(a),url(b),50%)','element(#foo)'],
}
for(const [property,values] of Object.entries(properties)){
 for(const value of [...values,'var(--value)','inherit'])add(`.a{${property}:${value}}`)
 add(`.a{${property}:${values[0]}!important}`)
}
for(const [prefix,property,value] of [['-moz-','tab-size','4'],['-moz-','appearance','textfield'],['-webkit-','backface-visibility','hidden']]) {
 add(`.a{${prefix}${property}:${value}}`)
 add(`.a{${prefix}${property}:${value};${property}:${value}}`)
 add(`.a{${prefix}${property}:${value};${property}:var(--value)}`)
 add(`.a{${prefix}${property}:${value}!important;${property}:${value}}`)
}
for(const property of ['width' ,'height','min-width','max-height','inline-size','block-size','min-block-size','flex-basis'])add(`.a{${property}:stretch}`)
for(const css of [
 '.a{-webkit-user-select:text;user-select:none}',
 '.a{-webkit-user-select:none;user-select:none}',
 '.a{user-select:none;user-select:text}',
 '.a{background:linear-gradient(red,blue) text}',
 '.a{background:text}',
 '.a{background:black image-set("a.png" 1x)}',
 '.a{user-select:foo;hyphens:foo;box-decoration-break:foo;text-size-adjust:foo;print-color-adjust:foo;text-decoration-skip-ink:foo}',
 '.a{mask-composite:foo;mask-mode:foo}',
 '.a{background-image:cross-fade(image-set("a" 1x),url(b),50%)}',
 '.a{--x:stretch;--y:image-set(url(a) 1x);--z:"text-decoration"}',
])add(css)
const conditions=[
 '(width >= 40rem)', '(width > 40rem)', '(width <= 40rem)', '(width < 40rem)', '(width = 40rem)',
 '(40rem > width)', '(40rem >= width)', '(40rem < width)', '(40rem <= width)', '(40rem = width)',
 '(1rem < width <= 40rem)', '(1rem <= width < 40rem)', '(40rem > width >= 1rem)',
 '(width > calc(40rem + 1px))', '(aspect-ratio > 16/9)', '(resolution >= 2dppx)',
 'not (width < 40rem)', 'not (width >= 40rem)', 'not (color)',
 '(width>4px) or (height<4px)', 'not ((width>4px) or (height<4px))',
 '(1rem < width <= 40rem) and (height>1px)', '(width>=1px) and ((color) or (width<2px))',
]
for(const condition of conditions){
 add(`@media ${condition}{.a{display:block}}`)
 add(`@media screen and ${condition}{.a{display:block}}`)
 add(`@container name ${condition}{.a{display:block}}`)
}
for(const css of [
 '@container style(--color: "wide > short"){.a{display:block}}',
 '@container style(--color: "wide \\" short"){.a{display:block}}',
 '@container (--foo){.a{display:block}}',
 '@media not print and (width>4px), (color){.a{display:block}}',
 '@media only screen and (width>=4px){.a{display:block}}',
 '@media not not (color){.a{display:block}}',
 '@media not (not (color)){.a{display:block}}',
 '@media (1 < 2){.a{display:block}}',
 '@media (>=){.a{display:block}}',
 '@custom-media --a (width>=1px); @media (--a){.a{display:block}}',
 '@custom-media --a (width>=1px), (color); @media (--a) and (height<2px){.a{display:block}}',
 '@custom-media --a screen and (width>=1px); @media (--a){.a{display:block}}',
 '@custom-media --a screen and (width>=1px); @media (--a) and (height<2px){.a{display:block}}',
 '@custom-media --a screen and (width>=1px); @media not (--a){.a{display:block}}',
 '@custom-media --a (--b); @custom-media --b (color); @media (--a){.a{display:block}}',
 '@custom-media --a true; @media (--a){.a{display:block}}',
 '@custom-media --a (color); @custom-media --a (width>4px); @media (--a){.a{display:block}}',
 '@media (--missing){.a{display:block}}',
 '@custom-media --a (--b); @custom-media --b (--a); @media (--a){.a{display:block}}',
 '@custom-media --a print, screen; @media (--a){.a{display:block}}',
 '@media all and (--missing){.a{display:block}}',
 '@media screen and (--missing){.a{display:block}}',
 '@media (color) and (--missing){.a{display:block}}',
 '@media not (--missing){.a{display:block}}',
 '@layer a{@media (--missing){.a{display:block}}}',
])add(css)
fs.writeFileSync('testdata/upstream/browser-edges.json',JSON.stringify({tailwind:require('tailwindcss/package.json').version,cases},null,2)+'\n')
console.log(`${cases.length} browser inputs, ${cases.filter(test=>test.error).length} expected errors`)
