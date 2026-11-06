#!/usr/bin/env python3
"""Generate compact Unicode collation tables from ICU's public data APIs."""
import hashlib
import json
import pathlib
import struct
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent.parent
prefix = pathlib.Path(sys.argv[1])
cache = root / '.cache'
cache.mkdir(exist_ok=True)
executable = cache / 'collation-data'
subprocess.run(['cc', str(root/'scripts/collation.c'), '-I'+str(prefix/'include'), '-L'+str(prefix/'lib'), '-licui18n', '-licuuc', '-o', str(executable)],check=True)
result = subprocess.run([str(executable)],check=True,capture_output=True,text=True)
print(result.stderr.strip())
if result.stderr.strip() != 'ICU 78.3, Unicode 17.0.0':
    raise SystemExit('Use the ICU 78.3 reference version, or update and verify the version pin first')
single, ccc, decomp, contractions = {}, {}, {}, {}
for line in result.stdout.splitlines():
    fields = line.split()
    kind = fields.pop(0)
    if kind == 'T':
        at = fields.index(':')
        key = ''.join(chr(int(x,16)) for x in fields[:at])
        contractions[key] = tuple(int(x,16) for x in fields[at+1:])
    else:
        cp = int(fields.pop(0),16)
        if kind == 'C': single[cp] = tuple(int(x,16) for x in fields)
        elif kind == 'K': ccc[cp] = int(fields[0])
        elif kind == 'D': decomp[cp] = ''.join(chr(int(x,16)) for x in fields).encode()

pool = bytearray()
values = {}
def store(data):
    data = bytes(data)
    if data in values: return values[data]
    offset = len(pool)
    pool.extend(data)
    values[data] = offset
    return offset
def weight(values):
    offset = store(b''.join(struct.pack('<I',v) for v in values))
    assert offset < 1<<24 and len(values)<256
    return offset | len(values)<<24

index = bytearray()
for cp, elems in sorted(single.items()):
    index.extend(struct.pack('<II',cp,weight(elems)))
normalization = bytearray()
for cp in sorted(ccc.keys() | decomp.keys()):
    text = decomp.get(cp,b'')
    normalization.extend(struct.pack('<III',cp,store(text),len(text) | ccc.get(cp,0)<<16))
contract = bytearray()
for key, elems in sorted(contractions.items()):
    text=key.encode()
    contract.extend(struct.pack('<III',store(text),len(text),weight(elems)))
header = struct.pack('<IIII',len(single),len(normalization)//12,len(contractions),16+len(index)+len(normalization)+len(contract))
data = header+index+normalization+contract+pool
target = root/'data/collation'
target.mkdir(exist_ok=True)
(target/'unicode.bin').write_bytes(data)
(target/'manifest.json').write_text(json.dumps({'icu':'78.3','unicode':'17.0.0','cldr':'48.0','locale':'en-US','source':'https://github.com/unicode-org/icu/tree/release-78.3','sha256':hashlib.sha256(data).hexdigest(),'single':len(single),'contractions':len(contractions),'normalization':len(normalization)//12,'bytes':len(data)},indent=2)+'\n')
print(f'{len(single)} single characters, {len(contractions)} contractions, {len(data)} bytes')
